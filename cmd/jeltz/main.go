package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/logging"
	"github.com/fabiant7t/jeltz/internal/xdg"
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
	xdgCfg, _ := xdg.ConfigDir()
	xdgData, _ := xdg.DataDir()

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

	// Build CLI overrides — only set fields that were explicitly provided.
	cli := config.CLIOverrides{
		Listen:   *listen,
		BasePath: *basePath,
		DataDir:  *dataDir,
		LogLevel: *logLevel,
	}
	// Flags with bool/int64 zero values are always "set" so we pass them if
	// non-default. For simplicity, always forward them as overrides.
	cli.InsecureUpstream = insecureUpstream
	cli.DumpTraffic = dumpTraffic
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

	logger.Info("jeltz starting",
		slog.String(logging.KeyComponent, "main"),
		slog.String("listen", cfg.Listen),
		slog.String("config_file", *configFile),
		slog.String("base_path", cfg.BasePath),
		slog.String("data_dir", cfg.DataDir),
		slog.String("xdg_config_dir", xdgCfg),
		slog.String("xdg_data_dir", xdgData),
		slog.Bool("insecure_upstream", cfg.InsecureUpstream),
		slog.Bool("dump_traffic", cfg.DumpTraffic),
		slog.Int64("max_body_bytes", cfg.MaxBodyBytes),
		slog.Int("rules", len(cfg.Rules)),
	)

	// Placeholder: proxy server will be started here in L2+.
	logger.Info("no proxy configured yet; exiting", slog.String(logging.KeyComponent, "main"))
}

func runCAPath() {
	dataDir, err := xdg.DataDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jeltz ca-path: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(filepath.Join(dataDir, "ca.crt.pem"))
}

func runCAInstallHint() {
	dataDir, err := xdg.DataDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jeltz ca-install-hint: %v\n", err)
		os.Exit(1)
	}
	caPath := filepath.Join(dataDir, "ca.crt.pem")
	fmt.Printf(`jeltz CA Certificate Installation Hints

CA certificate path: %s

macOS:
  sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s

Linux (Debian/Ubuntu):
  sudo cp %s /usr/local/share/ca-certificates/jeltz.crt
  sudo update-ca-certificates

Linux (Fedora/RHEL):
  sudo cp %s /etc/pki/ca-trust/source/anchors/jeltz.crt
  sudo update-ca-trust

Firefox (any OS):
  Open Settings → Privacy & Security → Certificates → View Certificates
  → Authorities → Import → select %s

Chrome/Chromium:
  Open Settings → Privacy and security → Security → Manage certificates
  → Authorities → Import → select %s
`, caPath, caPath, caPath, caPath, caPath, caPath)
}
