package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fabiant7t/jeltz/internal/config"
)

func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func TestLoad_NoFile(t *testing.T) {
	tmp := t.TempDir()
	cfg, err := config.Load("", tmp, tmp, config.CLIOverrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != "127.0.0.1:8080" {
		t.Errorf("default listen: got %q", cfg.Listen)
	}
	if cfg.BasePath != tmp {
		t.Errorf("base_path: got %q, want %q", cfg.BasePath, tmp)
	}
	if cfg.DataDir != tmp {
		t.Errorf("data_dir: got %q, want %q", cfg.DataDir, tmp)
	}
	if cfg.UpstreamDialTimeoutMS != 10000 {
		t.Errorf("default upstream_dial_timeout_ms: got %d", cfg.UpstreamDialTimeoutMS)
	}
	if cfg.MaxUpstreamRequestBodyBytes != 0 {
		t.Errorf("default max_upstream_request_body_bytes: got %d", cfg.MaxUpstreamRequestBodyBytes)
	}
	if cfg.UpstreamTLSHandshakeTimeoutMS != 10000 {
		t.Errorf("default upstream_tls_handshake_timeout_ms: got %d", cfg.UpstreamTLSHandshakeTimeoutMS)
	}
	if cfg.UpstreamResponseHeaderTimeoutMS != 30000 {
		t.Errorf("default upstream_response_header_timeout_ms: got %d", cfg.UpstreamResponseHeaderTimeoutMS)
	}
	if cfg.UpstreamIdleConnTimeoutMS != 60000 {
		t.Errorf("default upstream_idle_conn_timeout_ms: got %d", cfg.UpstreamIdleConnTimeoutMS)
	}
}

func TestLoad_BasicYAML(t *testing.T) {
	tmp := t.TempDir()
	p := writeConfig(t, tmp, `
version: 1
listen: "127.0.0.1:9999"
insecure_upstream: true
dump_traffic: false
max_body_bytes: 512
max_upstream_request_body_bytes: 4096
upstream_dial_timeout_ms: 1234
upstream_tls_handshake_timeout_ms: 2345
upstream_response_header_timeout_ms: 3456
upstream_idle_conn_timeout_ms: 4567
`)
	cfg, err := config.Load(p, tmp, tmp, config.CLIOverrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != "127.0.0.1:9999" {
		t.Errorf("listen: got %q", cfg.Listen)
	}
	if !cfg.InsecureUpstream {
		t.Error("insecure_upstream should be true")
	}
	if cfg.MaxBodyBytes != 512 {
		t.Errorf("max_body_bytes: got %d", cfg.MaxBodyBytes)
	}
	if cfg.MaxUpstreamRequestBodyBytes != 4096 {
		t.Errorf("max_upstream_request_body_bytes: got %d", cfg.MaxUpstreamRequestBodyBytes)
	}
	if cfg.UpstreamDialTimeoutMS != 1234 {
		t.Errorf("upstream_dial_timeout_ms: got %d", cfg.UpstreamDialTimeoutMS)
	}
	if cfg.UpstreamTLSHandshakeTimeoutMS != 2345 {
		t.Errorf("upstream_tls_handshake_timeout_ms: got %d", cfg.UpstreamTLSHandshakeTimeoutMS)
	}
	if cfg.UpstreamResponseHeaderTimeoutMS != 3456 {
		t.Errorf("upstream_response_header_timeout_ms: got %d", cfg.UpstreamResponseHeaderTimeoutMS)
	}
	if cfg.UpstreamIdleConnTimeoutMS != 4567 {
		t.Errorf("upstream_idle_conn_timeout_ms: got %d", cfg.UpstreamIdleConnTimeoutMS)
	}
}

func TestLoad_UnknownField(t *testing.T) {
	tmp := t.TempDir()
	p := writeConfig(t, tmp, `
version: 1
unknown_field: true
`)
	_, err := config.Load(p, tmp, tmp, config.CLIOverrides{})
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestLoad_WrongVersion(t *testing.T) {
	tmp := t.TempDir()
	p := writeConfig(t, tmp, `
version: 99
`)
	_, err := config.Load(p, tmp, tmp, config.CLIOverrides{})
	if err == nil {
		t.Fatal("expected error for wrong version, got nil")
	}
}

func TestLoad_CLIOverrides(t *testing.T) {
	tmp := t.TempDir()
	p := writeConfig(t, tmp, `
version: 1
listen: "127.0.0.1:9000"
`)
	insecure := true
	dial := int64(250)
	maxReq := int64(123)
	cfg, err := config.Load(p, tmp, tmp, config.CLIOverrides{
		Listen:                      "127.0.0.1:7777",
		InsecureUpstream:            &insecure,
		MaxUpstreamRequestBodyBytes: &maxReq,
		UpstreamDialTimeoutMS:       &dial,
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != "127.0.0.1:7777" {
		t.Errorf("CLI listen override: got %q", cfg.Listen)
	}
	if !cfg.InsecureUpstream {
		t.Error("CLI insecure_upstream override should be true")
	}
	if cfg.UpstreamDialTimeoutMS != 250 {
		t.Errorf("CLI upstream_dial_timeout_ms override: got %d", cfg.UpstreamDialTimeoutMS)
	}
	if cfg.MaxUpstreamRequestBodyBytes != 123 {
		t.Errorf("CLI max_upstream_request_body_bytes override: got %d", cfg.MaxUpstreamRequestBodyBytes)
	}
}

func TestLoad_RejectsNegativeTimeout(t *testing.T) {
	tmp := t.TempDir()
	p := writeConfig(t, tmp, `
version: 1
upstream_dial_timeout_ms: -1
`)
	_, err := config.Load(p, tmp, tmp, config.CLIOverrides{})
	if err == nil {
		t.Fatal("expected error for negative timeout")
	}
}

func TestLoad_RejectsNegativeMaxUpstreamRequestBodyBytes(t *testing.T) {
	tmp := t.TempDir()
	p := writeConfig(t, tmp, `
version: 1
max_upstream_request_body_bytes: -1
`)
	_, err := config.Load(p, tmp, tmp, config.CLIOverrides{})
	if err == nil {
		t.Fatal("expected error for negative max_upstream_request_body_bytes")
	}
}

func TestLoad_RelativeBasePath(t *testing.T) {
	tmp := t.TempDir()
	p := writeConfig(t, tmp, `
version: 1
base_path: "sub"
`)
	cfg, err := config.Load(p, tmp, tmp, config.CLIOverrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(tmp, "sub")
	if cfg.BasePath != want {
		t.Errorf("base_path: got %q, want %q", cfg.BasePath, want)
	}
}

func TestLoad_AbsoluteBasePath(t *testing.T) {
	tmp := t.TempDir()
	abs := "/tmp/jeltz-abs-test"
	p := writeConfig(t, tmp, "version: 1\nbase_path: "+abs+"\n")
	cfg, err := config.Load(p, tmp, tmp, config.CLIOverrides{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BasePath != abs {
		t.Errorf("base_path: got %q, want %q", cfg.BasePath, abs)
	}
}

func TestLoad_MissingConfigFile(t *testing.T) {
	tmp := t.TempDir()
	_, err := config.Load(filepath.Join(tmp, "nonexistent.yaml"), tmp, tmp, config.CLIOverrides{})
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}
