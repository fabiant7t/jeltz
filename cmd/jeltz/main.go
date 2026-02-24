package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fabiant7t/jeltz/internal/ca"
	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/logging"
	"github.com/fabiant7t/jeltz/internal/proxy"
	"github.com/fabiant7t/jeltz/internal/rules"
	"github.com/fabiant7t/jeltz/pkg/xdg"
)

// Build-time variables injected via -ldflags.
var (
	version     = "dev"
	buildDate   = ""
	gitRevision = ""
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "ca-path":
			runCAPath()
			return
		case "ca-install-hint":
			runCAInstallHint()
			return
		}
	}

	fs := flag.NewFlagSet("jeltz", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: jeltz [flags]\n\nSubcommands:\n  ca-path          Print CA certificate path\n  ca-install-hint  Print CA installation hints\n\nFlags:\n")
		fs.PrintDefaults()
	}

	// Resolve XDG dirs for flag defaults.
	xdgCfg, _ := xdg.ConfigDir("jeltz")
	xdgData, _ := xdg.DataDir("jeltz")

	defaultConfig := ""
	if xdgCfg != "" {
		candidate := filepath.Join(xdgCfg, "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			defaultConfig = candidate
		}
	}

	listen := fs.String("listen", "", "Proxy listen address (default 127.0.0.1:8080)")
	configFile := fs.String("config", defaultConfig, "Path to config.yaml")
	basePath := fs.String("base-path", "", "Base path for relative rule paths (default: XDG config dir)")
	dataDir := fs.String("data-dir", "", "Data directory (CA, certs; default: XDG data dir)")
	logLevel := fs.String("log-level", "info", "Log level: debug|info|warn|error")
	insecureUpstream := fs.Bool("insecure-upstream", false, "Skip TLS verification for upstream connections")
	dumpTraffic := fs.Bool("dump-traffic", false, "Log request/response headers and body snippets")
	maxBodyBytes := fs.Int64("max-body-bytes", 0, "Max body bytes to log when dump-traffic is enabled (default 1048576)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	logger, err := logging.New(*logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jeltz: %v\n", err)
		os.Exit(1)
	}

	cli := config.CLIOverrides{
		Listen:           *listen,
		BasePath:         *basePath,
		DataDir:          *dataDir,
		LogLevel:         *logLevel,
		InsecureUpstream: insecureUpstream,
		DumpTraffic:      dumpTraffic,
	}
	if *maxBodyBytes != 0 {
		cli.MaxBodyBytes = maxBodyBytes
	}

	cfg, err := config.Load(*configFile, xdgCfg, xdgData, cli)
	if err != nil {
		logger.Error("failed to load config",
			slog.String(logging.KeyComponent, "main"),
			slog.String(logging.KeyEvent, "config_error"),
			slog.String(logging.KeyError, err.Error()),
		)
		os.Exit(1)
	}

	// Load CA (creates on first run).
	caInstance, err := ca.Load(cfg.DataDir)
	if err != nil {
		logger.Error("failed to load CA",
			slog.String(logging.KeyComponent, "main"),
			slog.String(logging.KeyEvent, "config_error"),
			slog.String(logging.KeyError, err.Error()),
		)
		os.Exit(1)
	}

	// Compile rules.
	rs, err := rules.Compile(cfg.Rules, cfg.BasePath)
	if err != nil {
		logger.Error("failed to compile rules",
			slog.String(logging.KeyComponent, "main"),
			slog.String(logging.KeyEvent, "config_error"),
			slog.String(logging.KeyError, err.Error()),
		)
		os.Exit(1)
	}

	printBanner(cfg.Listen, *configFile, cfg.DataDir, caInstance.CertPath(), caInstance.P12Path(),
		len(cfg.Rules), *logLevel, cfg.InsecureUpstream, cfg.DumpTraffic)

	pipeline := proxy.NewPipeline(rs, cfg.InsecureUpstream)
	if cfg.DumpTraffic {
		pipeline = pipeline.WithDumpTraffic(cfg.MaxBodyBytes)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := proxy.New(cfg.Listen, logger, pipeline, caInstance)
	if err := srv.ListenAndServe(ctx); err != nil {
		logger.Error("server error",
			slog.String(logging.KeyComponent, "main"),
			slog.String(logging.KeyError, err.Error()),
		)
		os.Exit(1)
	}
}

func runCAPath() {
	dataDir, err := xdg.DataDir("jeltz")
	if err != nil {
		fmt.Fprintf(os.Stderr, "jeltz ca-path: %v\n", err)
		os.Exit(1)
	}
	caInstance, err := ca.Load(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jeltz ca-path: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(caInstance.CertPath())
}

func runCAInstallHint() {
	dataDir, err := xdg.DataDir("jeltz")
	if err != nil {
		fmt.Fprintf(os.Stderr, "jeltz ca-install-hint: %v\n", err)
		os.Exit(1)
	}
	caInstance, err := ca.Load(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jeltz ca-install-hint: %v\n", err)
		os.Exit(1)
	}
	caPath := caInstance.CertPath()
	p12Path := caInstance.P12Path()
	fmt.Printf(`jeltz CA Certificate Installation Hints

CA certificate path: %s
CA PKCS#12 bundle:   %s  (password: %s)

macOS:
  sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s

Linux (Debian/Ubuntu):
  sudo cp %s /usr/local/share/ca-certificates/jeltz.crt
  sudo update-ca-certificates

Linux (Fedora/RHEL):
  sudo cp %s /etc/pki/ca-trust/source/anchors/jeltz.crt
  sudo update-ca-trust

Windows:
  certutil -addstore Root %s
  (or double-click %s → Install Certificate → Local Machine → Trusted Root CAs)

Firefox (any OS):
  Open Settings → Privacy & Security → Certificates → View Certificates
  → Authorities → Import → select %s  (password: %s)

Chrome/Chromium (Linux/macOS uses the OS trust store above):
  Open Settings → Privacy and security → Security → Manage certificates
  → Authorities → Import → select %s
`, caPath, p12Path, ca.P12Password, caPath, caPath, caPath, p12Path, p12Path, p12Path, ca.P12Password, caPath)
}
