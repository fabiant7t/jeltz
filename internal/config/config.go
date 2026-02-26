// Package config handles jeltz configuration loading and validation.
package config

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strconv"
)

// Version is the only supported config version.
const Version = 1

// Config holds the fully resolved jeltz configuration.
type Config struct {
	Listen                          string
	BasePath                        string
	DataDir                         string
	LogLevel                        string
	InsecureUpstream                bool
	DumpTraffic                     bool
	MaxBodyBytes                    int64
	MaxUpstreamRequestBodyBytes     int64
	UpstreamDialTimeoutMS           int64
	UpstreamTLSHandshakeTimeoutMS   int64
	UpstreamResponseHeaderTimeoutMS int64
	UpstreamIdleConnTimeoutMS       int64
	Rules                           []RawRule
}

// RawRule holds a single rule as-loaded from YAML before type-specific parsing.
type RawRule struct {
	Type        string   `yaml:"type"`
	Match       RawMatch `yaml:"match"`
	Path        string   `yaml:"path,omitempty"`
	IndexFile   string   `yaml:"index_file,omitempty"`
	StatusCode  int      `yaml:"status_code,omitempty"`
	ContentType string   `yaml:"content_type,omitempty"`
	Search      string   `yaml:"search,omitempty"`
	Replace     string   `yaml:"replace,omitempty"`
	SearchMode  string   `yaml:"search_mode,omitempty"`
	Request     *RawOps  `yaml:"request,omitempty"`
	Response    *RawOps  `yaml:"response,omitempty"`
}

// RawMatch holds the raw match block from YAML.
type RawMatch struct {
	Methods []string `yaml:"methods"`
	Host    string   `yaml:"host"`
	Path    string   `yaml:"path"`
}

// RawOps holds ordered header operations.
type RawOps struct {
	Delete []RawDeleteOp `yaml:"delete"`
	Set    []RawSetOp    `yaml:"set"`
}

// RawDeleteOp is a single header delete operation (name-based or wildcard).
type RawDeleteOp struct {
	Name       string `yaml:"name,omitempty"`
	AnyName    bool   `yaml:"any_name,omitempty"`
	ValueRegex string `yaml:"value,omitempty"`
}

// RawSetOp is a single header set operation.
type RawSetOp struct {
	Name  string `yaml:"name"`
	Mode  string `yaml:"mode"` // replace | append
	Value string `yaml:"value"`
}

// yamlConfig mirrors the YAML schema exactly for strict decoding.
type yamlConfig struct {
	Version                         int       `yaml:"version"`
	Listen                          *string   `yaml:"listen"`
	BasePath                        *string   `yaml:"base_path"`
	DataDir                         *string   `yaml:"data_dir"`
	InsecureUpstream                *bool     `yaml:"insecure_upstream"`
	DumpTraffic                     *bool     `yaml:"dump_traffic"`
	MaxBodyBytes                    *int64    `yaml:"max_body_bytes"`
	MaxUpstreamRequestBodyBytes     *int64    `yaml:"max_upstream_request_body_bytes"`
	UpstreamDialTimeoutMS           *int64    `yaml:"upstream_dial_timeout_ms"`
	UpstreamTLSHandshakeTimeoutMS   *int64    `yaml:"upstream_tls_handshake_timeout_ms"`
	UpstreamResponseHeaderTimeoutMS *int64    `yaml:"upstream_response_header_timeout_ms"`
	UpstreamIdleConnTimeoutMS       *int64    `yaml:"upstream_idle_conn_timeout_ms"`
	Rules                           []RawRule `yaml:"rules"`
}

// CLIOverrides carries explicitly-set CLI flag values that take precedence
// over file/env config. Empty string means "not set by CLI".
type CLIOverrides struct {
	Listen                          string
	BasePath                        string
	DataDir                         string
	LogLevel                        string
	InsecureUpstream                *bool
	DumpTraffic                     *bool
	MaxBodyBytes                    *int64
	MaxUpstreamRequestBodyBytes     *int64
	UpstreamDialTimeoutMS           *int64
	UpstreamTLSHandshakeTimeoutMS   *int64
	UpstreamResponseHeaderTimeoutMS *int64
	UpstreamIdleConnTimeoutMS       *int64
}

// Load reads, validates, and resolves the configuration.
// configFile may be empty (no config file). xdgCfg and xdgData are the
// resolved XDG directories used for path defaults. cli overrides final values.
func Load(configFile, xdgCfg, xdgData string, cli CLIOverrides) (*Config, error) {
	cfg := &Config{
		Listen:                          "127.0.0.1:8080",
		BasePath:                        "",
		DataDir:                         "",
		LogLevel:                        "info",
		InsecureUpstream:                false,
		DumpTraffic:                     false,
		MaxBodyBytes:                    int64(1048576),
		MaxUpstreamRequestBodyBytes:     int64(0),
		UpstreamDialTimeoutMS:           int64(10000),
		UpstreamTLSHandshakeTimeoutMS:   int64(10000),
		UpstreamResponseHeaderTimeoutMS: int64(30000),
		UpstreamIdleConnTimeoutMS:       int64(60000),
	}

	if err := applyEnv(cfg); err != nil {
		return nil, err
	}

	var rawYAML []byte
	var yc yamlConfig

	if configFile != "" {
		if _, err := os.Stat(configFile); err != nil {
			return nil, fmt.Errorf("config file %q not found: %w", configFile, err)
		}
		var readErr error
		rawYAML, readErr = os.ReadFile(configFile)
		if readErr != nil {
			return nil, fmt.Errorf("reading config file: %w", readErr)
		}
		dec := yaml.NewDecoder(bytes.NewReader(rawYAML))
		dec.KnownFields(true)
		if err := dec.Decode(&yc); err != nil {
			return nil, fmt.Errorf("config validation: %w", err)
		}
		if yc.Version != Version {
			return nil, fmt.Errorf("config version must be %d, got %d", Version, yc.Version)
		}
		applyFileConfig(cfg, yc)
		cfg.Rules = yc.Rules
	}

	// Apply CLI overrides (highest precedence).
	if cli.Listen != "" {
		cfg.Listen = cli.Listen
	}
	if cli.LogLevel != "" {
		cfg.LogLevel = cli.LogLevel
	}
	if cli.InsecureUpstream != nil {
		cfg.InsecureUpstream = *cli.InsecureUpstream
	}
	if cli.DumpTraffic != nil {
		cfg.DumpTraffic = *cli.DumpTraffic
	}
	if cli.MaxBodyBytes != nil {
		cfg.MaxBodyBytes = *cli.MaxBodyBytes
	}
	if cli.MaxUpstreamRequestBodyBytes != nil {
		cfg.MaxUpstreamRequestBodyBytes = *cli.MaxUpstreamRequestBodyBytes
	}
	if cli.UpstreamDialTimeoutMS != nil {
		cfg.UpstreamDialTimeoutMS = *cli.UpstreamDialTimeoutMS
	}
	if cli.UpstreamTLSHandshakeTimeoutMS != nil {
		cfg.UpstreamTLSHandshakeTimeoutMS = *cli.UpstreamTLSHandshakeTimeoutMS
	}
	if cli.UpstreamResponseHeaderTimeoutMS != nil {
		cfg.UpstreamResponseHeaderTimeoutMS = *cli.UpstreamResponseHeaderTimeoutMS
	}
	if cli.UpstreamIdleConnTimeoutMS != nil {
		cfg.UpstreamIdleConnTimeoutMS = *cli.UpstreamIdleConnTimeoutMS
	}

	if cfg.MaxUpstreamRequestBodyBytes < 0 ||
		cfg.UpstreamDialTimeoutMS < 0 ||
		cfg.UpstreamTLSHandshakeTimeoutMS < 0 ||
		cfg.UpstreamResponseHeaderTimeoutMS < 0 ||
		cfg.UpstreamIdleConnTimeoutMS < 0 {
		return nil, fmt.Errorf("request/timeout values must be >= 0")
	}

	cfg.BasePath = resolveBasePath(cfg.BasePath, xdgCfg)
	cfg.DataDir = resolveDataDir(cfg.DataDir, xdgData)

	return cfg, nil
}

func applyFileConfig(cfg *Config, yc yamlConfig) {
	if yc.Listen != nil {
		cfg.Listen = *yc.Listen
	}
	if yc.BasePath != nil {
		cfg.BasePath = *yc.BasePath
	}
	if yc.DataDir != nil {
		cfg.DataDir = *yc.DataDir
	}
	if yc.InsecureUpstream != nil {
		cfg.InsecureUpstream = *yc.InsecureUpstream
	}
	if yc.DumpTraffic != nil {
		cfg.DumpTraffic = *yc.DumpTraffic
	}
	if yc.MaxBodyBytes != nil {
		cfg.MaxBodyBytes = *yc.MaxBodyBytes
	}
	if yc.MaxUpstreamRequestBodyBytes != nil {
		cfg.MaxUpstreamRequestBodyBytes = *yc.MaxUpstreamRequestBodyBytes
	}
	if yc.UpstreamDialTimeoutMS != nil {
		cfg.UpstreamDialTimeoutMS = *yc.UpstreamDialTimeoutMS
	}
	if yc.UpstreamTLSHandshakeTimeoutMS != nil {
		cfg.UpstreamTLSHandshakeTimeoutMS = *yc.UpstreamTLSHandshakeTimeoutMS
	}
	if yc.UpstreamResponseHeaderTimeoutMS != nil {
		cfg.UpstreamResponseHeaderTimeoutMS = *yc.UpstreamResponseHeaderTimeoutMS
	}
	if yc.UpstreamIdleConnTimeoutMS != nil {
		cfg.UpstreamIdleConnTimeoutMS = *yc.UpstreamIdleConnTimeoutMS
	}
}

func applyEnv(cfg *Config) error {
	if v, ok := os.LookupEnv("JELTZ_LISTEN"); ok {
		cfg.Listen = v
	}
	if v, ok := os.LookupEnv("JELTZ_BASE_PATH"); ok {
		cfg.BasePath = v
	}
	if v, ok := os.LookupEnv("JELTZ_DATA_DIR"); ok {
		cfg.DataDir = v
	}
	if v, ok := os.LookupEnv("JELTZ_INSECURE_UPSTREAM"); ok {
		p, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid JELTZ_INSECURE_UPSTREAM: %w", err)
		}
		cfg.InsecureUpstream = p
	}
	if v, ok := os.LookupEnv("JELTZ_DUMP_TRAFFIC"); ok {
		p, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid JELTZ_DUMP_TRAFFIC: %w", err)
		}
		cfg.DumpTraffic = p
	}
	if v, ok := os.LookupEnv("JELTZ_MAX_BODY_BYTES"); ok {
		p, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid JELTZ_MAX_BODY_BYTES: %w", err)
		}
		cfg.MaxBodyBytes = p
	}
	if v, ok := os.LookupEnv("JELTZ_MAX_UPSTREAM_REQUEST_BODY_BYTES"); ok {
		p, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid JELTZ_MAX_UPSTREAM_REQUEST_BODY_BYTES: %w", err)
		}
		cfg.MaxUpstreamRequestBodyBytes = p
	}
	if v, ok := os.LookupEnv("JELTZ_UPSTREAM_DIAL_TIMEOUT_MS"); ok {
		p, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid JELTZ_UPSTREAM_DIAL_TIMEOUT_MS: %w", err)
		}
		cfg.UpstreamDialTimeoutMS = p
	}
	if v, ok := os.LookupEnv("JELTZ_UPSTREAM_TLS_HANDSHAKE_TIMEOUT_MS"); ok {
		p, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid JELTZ_UPSTREAM_TLS_HANDSHAKE_TIMEOUT_MS: %w", err)
		}
		cfg.UpstreamTLSHandshakeTimeoutMS = p
	}
	if v, ok := os.LookupEnv("JELTZ_UPSTREAM_RESPONSE_HEADER_TIMEOUT_MS"); ok {
		p, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid JELTZ_UPSTREAM_RESPONSE_HEADER_TIMEOUT_MS: %w", err)
		}
		cfg.UpstreamResponseHeaderTimeoutMS = p
	}
	if v, ok := os.LookupEnv("JELTZ_UPSTREAM_IDLE_CONN_TIMEOUT_MS"); ok {
		p, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid JELTZ_UPSTREAM_IDLE_CONN_TIMEOUT_MS: %w", err)
		}
		cfg.UpstreamIdleConnTimeoutMS = p
	}
	return nil
}

// resolveBasePath returns the absolute effective base path.
// Empty or "." → xdgCfg. Relative → resolve against xdgCfg. Absolute → as-is.
func resolveBasePath(p, xdgCfg string) string {
	if p == "" || p == "." {
		return xdgCfg
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(xdgCfg, p)
}

// resolveDataDir returns the absolute effective data directory.
// Empty → xdgData. Relative → resolve against xdgData. Absolute → as-is.
func resolveDataDir(p, xdgData string) string {
	if p == "" {
		return xdgData
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(xdgData, p)
}
