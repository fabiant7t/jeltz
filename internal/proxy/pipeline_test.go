package proxy_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/proxy"
	"github.com/fabiant7t/jeltz/internal/rules"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makeRuleSet(t *testing.T, rawRules []config.RawRule, basePath string) *rules.RuleSet {
	t.Helper()
	rs, err := rules.Compile(rawRules, basePath)
	if err != nil {
		t.Fatalf("Compile rules: %v", err)
	}
	return rs
}

// upstreamPort extracts the port string from a test server address.
func upstreamPort(s *httptest.Server) string {
	addr := s.Listener.Addr().String()
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i+1:]
		}
	}
	return ""
}

func TestPipeline_MapLocal_ServesFile(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "static")
	_ = os.MkdirAll(subDir, 0o755)
	_ = os.WriteFile(filepath.Join(subDir, "hello.txt"), []byte("hello world"), 0o644)

	rs := makeRuleSet(t, []config.RawRule{
		{
			Type:  "map_local",
			Match: config.RawMatch{Host: `^example\.com$`, Path: `^/static/`},
			Path:  subDir,
		},
	}, dir)

	p := proxy.NewPipeline(rs, false)
	fc := &proxy.FlowContext{
		Logger:     testLogger(),
		ClientAddr: "127.0.0.1:1234",
		Proto:      "http/1.1",
		Scheme:     "https",
		Host:       "example.com",
		Method:     "GET",
		Path:       "/static/hello.txt",
		Header:     make(http.Header),
	}

	result, err := p.Run(fc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Errorf("status: got %d, want 200", result.Status)
	}
	if result.Source != "local" {
		t.Errorf("source: got %q, want local", result.Source)
	}
	body, _ := io.ReadAll(result.Body)
	if string(body) != "hello world" {
		t.Errorf("body: got %q", string(body))
	}
}

func TestPipeline_ResponseHeaderRule(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "static")
	_ = os.MkdirAll(subDir, 0o755)
	_ = os.WriteFile(filepath.Join(subDir, "f.txt"), []byte("data"), 0o644)

	rs := makeRuleSet(t, []config.RawRule{
		{
			Type:  "header",
			Match: config.RawMatch{Host: `^example\.com$`, Path: `.*`},
			Response: &config.RawOps{
				Set: []config.RawSetOp{{Name: "X-Test", Mode: "replace", Value: "1"}},
			},
		},
		{
			Type:  "map_local",
			Match: config.RawMatch{Host: `^example\.com$`, Path: `^/static/`},
			Path:  subDir,
		},
	}, dir)

	p := proxy.NewPipeline(rs, false)
	fc := &proxy.FlowContext{
		Logger: testLogger(), Scheme: "https", Host: "example.com",
		Method: "GET", Path: "/static/f.txt", Header: make(http.Header),
	}

	result, err := p.Run(fc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Headers.Get("X-Test") != "1" {
		t.Errorf("X-Test: got %q, want 1", result.Headers.Get("X-Test"))
	}
}

func TestPipeline_RequestHeaderTransform_Upstream(t *testing.T) {
	// Upstream echoes back the received X-Debug header.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Got-Debug", r.Header.Get("X-Debug"))
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	port := upstreamPort(upstream)

	rs := makeRuleSet(t, []config.RawRule{
		{
			Type:  "header",
			Match: config.RawMatch{Host: `^127\.0\.0\.1$`, Path: `.*`},
			Request: &config.RawOps{
				Set: []config.RawSetOp{{Name: "X-Debug", Mode: "replace", Value: "injected"}},
			},
		},
	}, t.TempDir())

	p := proxy.NewPipeline(rs, false)
	fc := &proxy.FlowContext{
		Logger:     testLogger(),
		ClientAddr: "127.0.0.1:1234",
		Proto:      "http/1.1",
		Scheme:     "http",
		Host:       "127.0.0.1",
		Port:       port,
		Method:     "GET",
		Path:       "/",
		Header:     make(http.Header),
	}

	result, err := p.Run(fc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Errorf("status: got %d", result.Status)
	}
	if result.Headers.Get("X-Got-Debug") != "injected" {
		t.Errorf("upstream received X-Debug: got %q, want injected", result.Headers.Get("X-Got-Debug"))
	}
}

func TestPipeline_MapLocal_404WhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "static")
	_ = os.MkdirAll(subDir, 0o755)

	rs := makeRuleSet(t, []config.RawRule{
		{
			Type:  "map_local",
			Match: config.RawMatch{Host: `^example\.com$`, Path: `^/static/`},
			Path:  subDir,
		},
	}, dir)

	p := proxy.NewPipeline(rs, false)
	fc := &proxy.FlowContext{
		Logger: testLogger(), Scheme: "https", Host: "example.com",
		Method: "GET", Path: "/static/missing.txt", Header: make(http.Header),
	}
	result, err := p.Run(fc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", result.Status)
	}
}

func TestPipeline_MapLocalResponseOps_AfterGlobalResponse(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644)

	rs := makeRuleSet(t, []config.RawRule{
		// Global response rule.
		{
			Type:  "header",
			Match: config.RawMatch{Host: `.*`, Path: `.*`},
			Response: &config.RawOps{
				Set: []config.RawSetOp{{Name: "X-Global", Mode: "replace", Value: "g"}},
			},
		},
		// map-local rule with its own response op.
		{
			Type:  "map_local",
			Match: config.RawMatch{Host: `^example\.com$`, Path: `^/f`},
			Path:  filepath.Join(dir, "f.txt"),
			Response: &config.RawOps{
				Set: []config.RawSetOp{{Name: "X-Local", Mode: "replace", Value: "l"}},
			},
		},
	}, dir)

	p := proxy.NewPipeline(rs, false)
	fc := &proxy.FlowContext{
		Logger: testLogger(), Scheme: "https", Host: "example.com",
		Method: "GET", Path: "/f", Header: make(http.Header),
	}
	result, err := p.Run(fc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Headers.Get("X-Global") != "g" {
		t.Errorf("X-Global: got %q", result.Headers.Get("X-Global"))
	}
	if result.Headers.Get("X-Local") != "l" {
		t.Errorf("X-Local: got %q", result.Headers.Get("X-Local"))
	}
}

func TestWriteResponse(t *testing.T) {
	fc := &proxy.FlowContext{
		Logger:     testLogger(),
		ClientAddr: "127.0.0.1:1234",
		Proto:      "http/1.1",
		Scheme:     "https",
		Host:       "example.com",
		Method:     "GET",
		Path:       "/",
		Header:     make(http.Header),
	}
	result := &proxy.ResponseResult{
		Status:  http.StatusOK,
		Headers: http.Header{"X-Foo": []string{"bar"}},
		Body:    io.NopCloser(bytes.NewReader([]byte("ok"))),
		Source:  "local",
	}
	w := httptest.NewRecorder()
	proxy.WriteResponse(w, result, fc, time.Now())
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("body: got %q", w.Body.String())
	}
	if w.Header().Get("X-Foo") != "bar" {
		t.Errorf("X-Foo: got %q", w.Header().Get("X-Foo"))
	}
}
