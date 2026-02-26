package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fabiant7t/jeltz/internal/ca"
	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/logging"
	"github.com/fabiant7t/jeltz/internal/logstream"
	"github.com/fabiant7t/jeltz/internal/proxy"
	"github.com/fabiant7t/jeltz/internal/rules"
	"github.com/fabiant7t/jeltz/internal/tui"
	"github.com/fabiant7t/jeltz/pkg/xdg"
)

// Build-time variables injected via -ldflags.
var (
	version     = "dev"
	buildDate   = ""
	gitRevision = ""
)

func main() {
	subcommand, hasSubcommand, err := parseSubcommand(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "jeltz: %v\n", err)
		os.Exit(1)
	}
	if hasSubcommand {
		runSubcommand(subcommand)
		return
	}

	fs := flag.NewFlagSet("jeltz", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: jeltz [flags]\n\nSubcommands:\n  ca-path          Print CA certificate path\n  ca-p12-path      Print CA bundle path\n  ca-install-hint  Print CA installation hints\n\nFlags:\n")
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
	ui := fs.Bool("ui", false, "Run an interactive terminal UI for live log viewing")
	maxBodyBytes := fs.Int64("max-body-bytes", 0, "Max body bytes to log when dump-traffic is enabled (default 1048576)")
	maxUpstreamRequestBodyBytes := fs.Int64("max-upstream-request-body-bytes", 0, "Max upstream request body bytes (0 = unlimited)")
	upstreamDialTimeoutMS := fs.Int64("upstream-dial-timeout-ms", 0, "Upstream TCP dial timeout in milliseconds (default 10000)")
	upstreamTLSHandshakeTimeoutMS := fs.Int64("upstream-tls-handshake-timeout-ms", 0, "Upstream TLS handshake timeout in milliseconds (default 10000)")
	upstreamResponseHeaderTimeoutMS := fs.Int64("upstream-response-header-timeout-ms", 0, "Upstream response header timeout in milliseconds (default 30000)")
	upstreamIdleConnTimeoutMS := fs.Int64("upstream-idle-conn-timeout-ms", 0, "Upstream idle connection timeout in milliseconds (default 60000)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	var stream *logstream.Stream
	var logger *slog.Logger
	if *ui {
		level, parseErr := logging.ParseLevel(*logLevel)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "jeltz: %v\n", parseErr)
			os.Exit(1)
		}
		stream = logstream.New(4096)
		logger = slog.New(stream.Handler(level))
	} else {
		logger, err = logging.New(*logLevel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jeltz: %v\n", err)
			os.Exit(1)
		}
	}

	cli := config.CLIOverrides{
		Listen:           *listen,
		BasePath:         *basePath,
		DataDir:          *dataDir,
		LogLevel:         *logLevel,
		InsecureUpstream: boolFlagPtrIfSet(fs, "insecure-upstream", *insecureUpstream),
		DumpTraffic:      boolFlagPtrIfSet(fs, "dump-traffic", *dumpTraffic),
		MaxBodyBytes:     int64FlagPtrIfSet(fs, "max-body-bytes", *maxBodyBytes),
		MaxUpstreamRequestBodyBytes: int64FlagPtrIfSet(fs,
			"max-upstream-request-body-bytes", *maxUpstreamRequestBodyBytes),
		UpstreamDialTimeoutMS: int64FlagPtrIfSet(fs,
			"upstream-dial-timeout-ms", *upstreamDialTimeoutMS),
		UpstreamTLSHandshakeTimeoutMS: int64FlagPtrIfSet(fs,
			"upstream-tls-handshake-timeout-ms", *upstreamTLSHandshakeTimeoutMS),
		UpstreamResponseHeaderTimeoutMS: int64FlagPtrIfSet(fs,
			"upstream-response-header-timeout-ms", *upstreamResponseHeaderTimeoutMS),
		UpstreamIdleConnTimeoutMS: int64FlagPtrIfSet(fs,
			"upstream-idle-conn-timeout-ms", *upstreamIdleConnTimeoutMS),
	}

	cfg, err := config.Load(*configFile, xdgCfg, xdgData, cli)
	if err != nil {
		logger.Error("failed to load config",
			slog.String(logging.KeyComponent, "main"),
			slog.String(logging.KeyEvent, "config_error"),
			slog.String(logging.KeyError, err.Error()),
		)
		printStartupFailure("failed to load config", err)
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
		printStartupFailure("failed to load CA", err)
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
		printStartupFailure("failed to compile rules", err)
		os.Exit(1)
	}

	if !*ui {
		printBanner(cfg.Listen, *configFile, cfg.DataDir, caInstance.CertPath(), caInstance.P12Path(),
			len(cfg.Rules), *logLevel, cfg.InsecureUpstream, cfg.DumpTraffic)
	}

	pipeline := proxy.NewPipeline(rs, cfg.InsecureUpstream)
	pipeline = pipeline.WithMaxUpstreamRequestBodyBytes(cfg.MaxUpstreamRequestBodyBytes)
	pipeline = pipeline.WithTransportTimeouts(proxy.TransportTimeouts{
		DialTimeout:           time.Duration(cfg.UpstreamDialTimeoutMS) * time.Millisecond,
		TLSHandshakeTimeout:   time.Duration(cfg.UpstreamTLSHandshakeTimeoutMS) * time.Millisecond,
		ResponseHeaderTimeout: time.Duration(cfg.UpstreamResponseHeaderTimeoutMS) * time.Millisecond,
		IdleConnTimeout:       time.Duration(cfg.UpstreamIdleConnTimeoutMS) * time.Millisecond,
	})
	if cfg.DumpTraffic {
		pipeline = pipeline.WithDumpTraffic(cfg.MaxBodyBytes)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := proxy.New(cfg.Listen, logger, pipeline, caInstance)
	if !*ui {
		if err := srv.ListenAndServe(ctx); err != nil {
			logger.Error("server error",
				slog.String(logging.KeyComponent, "main"),
				slog.String(logging.KeyError, err.Error()),
			)
			os.Exit(1)
		}
		return
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(ctx)
		stop()
	}()

	if err := tui.Run(ctx, tui.Config{
		ListenAddr: cfg.Listen,
		Events:     stream.Events(),
		Dropped:    stream.Dropped,
		Stop:       stop,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "jeltz ui: %v\n", err)
	}
	if err := <-errCh; err != nil {
		fmt.Fprintf(os.Stderr, "jeltz: server error: %v\n", err)
		os.Exit(1)
	}
}

func parseSubcommand(args []string) (string, bool, error) {
	if len(args) == 0 {
		return "", false, nil
	}
	first := args[0]
	if strings.HasPrefix(first, "-") {
		return "", false, nil
	}
	switch first {
	case "ca-path", "ca-p12-path", "ca-install-hint":
		return first, true, nil
	default:
		return "", false, fmt.Errorf("unknown subcommand %q", first)
	}
}

func runSubcommand(name string) {
	switch name {
	case "ca-path":
		runCAPath()
	case "ca-p12-path":
		runCAP12Path()
	case "ca-install-hint":
		runCAInstallHint()
	default:
		fmt.Fprintf(os.Stderr, "jeltz: unknown subcommand %q\n", name)
		os.Exit(1)
	}
}

func boolFlagPtrIfSet(fs *flag.FlagSet, name string, value bool) *bool {
	if !flagWasSet(fs, name) {
		return nil
	}
	v := value
	return &v
}

func int64FlagPtrIfSet(fs *flag.FlagSet, name string, value int64) *int64 {
	if !flagWasSet(fs, name) {
		return nil
	}
	v := value
	return &v
}

func printStartupFailure(message string, err error) {
	fmt.Fprintf(os.Stderr, "jeltz: %s: %v\n", message, err)
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
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

func runCAP12Path() {
	dataDir, err := xdg.DataDir("jeltz")
	if err != nil {
		fmt.Fprintf(os.Stderr, "jeltz ca-p12-path: %v\n", err)
		os.Exit(1)
	}
	caInstance, err := ca.Load(dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jeltz ca-p12-path: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(caInstance.P12Path())
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
  → Authorities → Import → select %s

Chrome/Chromium (Linux/macOS uses the OS trust store above):
  Open Settings → Privacy and security → Security → Manage certificates
  → Authorities → Import → select %s
`, caPath, p12Path, ca.P12Password, caPath, caPath, caPath, p12Path, p12Path, caPath, caPath)
}
