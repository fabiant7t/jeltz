// Package config handles jeltz configuration loading and validation.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Version is the only supported config version.
const Version = 1

// Config holds the fully resolved jeltz configuration.
type Config struct {
	Listen           string
	BasePath         string
	DataDir          string
	LogLevel         string
	InsecureUpstream bool
	DumpTraffic      bool
	MaxBodyBytes     int64
	Rules            []RawRule
}

// RawRule holds a single rule as-loaded from YAML before type-specific parsing.
type RawRule struct {
	Type     string      `yaml:"type"`
	Match    RawMatch    `yaml:"match"`
	Path     string      `yaml:"path,omitempty"`
	IndexFile string     `yaml:"index_file,omitempty"`
	StatusCode int       `yaml:"status_code,omitempty"`
	ContentType string   `yaml:"content_type,omitempty"`
	Request  *RawOps     `yaml:"request,omitempty"`
	Response *RawOps     `yaml:"response,omitempty"`
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
	ValueRegex string `yaml:"value_regex,omitempty"`
}

// RawSetOp is a single header set operation.
type RawSetOp struct {
	Name  string `yaml:"name"`
	Mode  string `yaml:"mode"` // replace | append
	Value string `yaml:"value"`
}

// yamlConfig mirrors the YAML schema exactly for strict decoding.
type yamlConfig struct {
	Version          int       `yaml:"version"`
	Listen           string    `yaml:"listen"`
	BasePath         string    `yaml:"base_path"`
	DataDir          string    `yaml:"data_dir"`
	InsecureUpstream bool      `yaml:"insecure_upstream"`
	DumpTraffic      bool      `yaml:"dump_traffic"`
	MaxBodyBytes     int64     `yaml:"max_body_bytes"`
	Rules            []RawRule `yaml:"rules"`
}

// CLIOverrides carries explicitly-set CLI flag values that take precedence
// over file/env config. Empty string means "not set by CLI".
type CLIOverrides struct {
	Listen           string
	BasePath         string
	DataDir          string
	LogLevel         string
	InsecureUpstream *bool
	DumpTraffic      *bool
	MaxBodyBytes     *int64
}

// Load reads, validates, and resolves the configuration.
// configFile may be empty (no config file). xdgCfg and xdgData are the
// resolved XDG directories used for path defaults. cli overrides final values.
func Load(configFile, xdgCfg, xdgData string, cli CLIOverrides) (*Config, error) {
	v := viper.New()

	// Defaults.
	v.SetDefault("listen", "127.0.0.1:8080")
	v.SetDefault("base_path", "")
	v.SetDefault("data_dir", "")
	v.SetDefault("insecure_upstream", false)
	v.SetDefault("dump_traffic", false)
	v.SetDefault("max_body_bytes", int64(1048576))
	v.SetDefault("rules", []interface{}{})

	// Env vars (JELTZ_ prefix).
	v.SetEnvPrefix("JELTZ")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var rawYAML []byte

	if configFile != "" {
		if _, err := os.Stat(configFile); err != nil {
			return nil, fmt.Errorf("config file %q not found: %w", configFile, err)
		}
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		var readErr error
		rawYAML, readErr = os.ReadFile(configFile)
		if readErr != nil {
			return nil, fmt.Errorf("re-reading config file for validation: %w", readErr)
		}
	}

	// Strict YAML validation via yaml.v3.
	if len(rawYAML) > 0 {
		dec := yaml.NewDecoder(bytes.NewReader(rawYAML))
		dec.KnownFields(true)
		var yc yamlConfig
		if err := dec.Decode(&yc); err != nil {
			return nil, fmt.Errorf("config validation: %w", err)
		}
		if yc.Version != Version {
			return nil, fmt.Errorf("config version must be %d, got %d", Version, yc.Version)
		}
	}

	// Build Config from viper values.
	cfg := &Config{
		Listen:           v.GetString("listen"),
		BasePath:         v.GetString("base_path"),
		DataDir:          v.GetString("data_dir"),
		LogLevel:         "info",
		InsecureUpstream: v.GetBool("insecure_upstream"),
		DumpTraffic:      v.GetBool("dump_traffic"),
		MaxBodyBytes:     v.GetInt64("max_body_bytes"),
	}

	// Parse rules via yaml.v3 for proper typing (viper loses type info).
	if len(rawYAML) > 0 {
		var yc yamlConfig
		if err := yaml.Unmarshal(rawYAML, &yc); err != nil {
			return nil, fmt.Errorf("parsing rules: %w", err)
		}
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

	// Resolve base_path.
	effectiveBase := v.GetString("base_path")
	if cli.BasePath != "" {
		effectiveBase = cli.BasePath
	}
	cfg.BasePath = resolveBasePath(effectiveBase, xdgCfg)

	// Resolve data_dir.
	effectiveData := v.GetString("data_dir")
	if cli.DataDir != "" {
		effectiveData = cli.DataDir
	}
	cfg.DataDir = resolveDataDir(effectiveData, xdgData)

	return cfg, nil
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
