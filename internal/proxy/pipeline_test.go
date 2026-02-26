package proxy_test

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
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

func TestPipeline_MapLocal_ServesLargeFile(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "static")
	_ = os.MkdirAll(subDir, 0o755)

	want := bytes.Repeat([]byte("0123456789abcdef"), 64*1024) // 1 MiB
	_ = os.WriteFile(filepath.Join(subDir, "large.bin"), want, 0o644)

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
		Path:       "/static/large.bin",
		Header:     make(http.Header),
	}

	result, err := p.Run(fc)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("status: got %d, want 200", result.Status)
	}
	got, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("body mismatch: got %d bytes, want %d", len(got), len(want))
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

func TestPipeline_UpstreamResponseHeaderTimeoutReturns502(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Keep the connection open without writing headers so the
		// response header timeout path is exercised.
		time.Sleep(200 * time.Millisecond)
	}()

	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	p := proxy.NewPipeline(nil, false).WithTransportTimeouts(proxy.TransportTimeouts{
		DialTimeout:           100 * time.Millisecond,
		TLSHandshakeTimeout:   100 * time.Millisecond,
		ResponseHeaderTimeout: 50 * time.Millisecond,
		IdleConnTimeout:       100 * time.Millisecond,
	})

	start := time.Now()
	result, runErr := p.Run(&proxy.FlowContext{
		Logger: testLogger(), Scheme: "http", Host: host, Port: port,
		Method: "GET", Path: "/", Header: make(http.Header),
	})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if result.Status != http.StatusBadGateway {
		t.Fatalf("status: got %d, want %d", result.Status, http.StatusBadGateway)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("timeout handling took too long: %v", d)
	}
}

func TestPipeline_DumpTraffic_DoesNotTruncateBody(t *testing.T) {
	payload := bytes.Repeat([]byte("abcdef0123456789"), 64*1024) // 1 MiB

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	host, port, err := net.SplitHostPort(upstream.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	p := proxy.NewPipeline(nil, false).WithDumpTraffic(1024)
	result, runErr := p.Run(&proxy.FlowContext{
		Logger: testLogger(), Scheme: "http", Host: host, Port: port,
		Method: "GET", Path: "/", Header: make(http.Header),
	})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("status: got %d, want %d", result.Status, http.StatusOK)
	}

	got, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("body mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}

func TestPipeline_UpstreamRequestBodyLimit_WithinLimit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	}))
	defer upstream.Close()

	host, port, err := net.SplitHostPort(upstream.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	p := proxy.NewPipeline(nil, false).WithMaxUpstreamRequestBodyBytes(8)
	result, runErr := p.Run(&proxy.FlowContext{
		Logger: testLogger(), Scheme: "http", Host: host, Port: port,
		Method: "POST", Path: "/", Header: make(http.Header),
		Body: io.NopCloser(bytes.NewReader([]byte("12345678"))),
	})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("status: got %d, want %d", result.Status, http.StatusOK)
	}
	got, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != "12345678" {
		t.Fatalf("body: got %q", string(got))
	}
}

func TestPipeline_UpstreamRequestBodyLimit_ExceededReturns413(t *testing.T) {
	var hit int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hit, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	host, port, err := net.SplitHostPort(upstream.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	p := proxy.NewPipeline(nil, false).WithMaxUpstreamRequestBodyBytes(8)
	result, runErr := p.Run(&proxy.FlowContext{
		Logger: testLogger(), Scheme: "http", Host: host, Port: port,
		Method: "POST", Path: "/", Header: make(http.Header),
		Body: io.NopCloser(bytes.NewReader([]byte("123456789"))),
	})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if result.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: got %d, want %d", result.Status, http.StatusRequestEntityTooLarge)
	}
	if atomic.LoadInt32(&hit) != 0 {
		t.Fatalf("upstream should not be called, hit=%d", hit)
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

func TestPipeline_BodyReplace_RegexAndConfigOrder(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("foo1 foo2"))
	}))
	defer upstream.Close()

	host, port, err := net.SplitHostPort(upstream.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	rs := makeRuleSet(t, []config.RawRule{
		{
			Type:    "body_replace",
			Match:   config.RawMatch{Host: "^" + host + "$", Path: `^/`},
			Search:  `foo(\d+)`,
			Replace: `bar-$1`,
		},
		{
			Type:    "body_replace",
			Match:   config.RawMatch{Host: "^" + host + "$", Path: `^/`},
			Search:  "bar-",
			Replace: "id-",
		},
	}, t.TempDir())

	p := proxy.NewPipeline(rs, false)
	result, runErr := p.Run(&proxy.FlowContext{
		Logger: testLogger(), Scheme: "http", Host: host, Port: port,
		Method: "GET", Path: "/", Header: make(http.Header),
	})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("status: got %d, want %d", result.Status, http.StatusOK)
	}

	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got, want := string(body), "id-1 id-2"; got != want {
		t.Fatalf("body: got %q, want %q", got, want)
	}
	if got, want := result.Headers.Get("Content-Length"), "9"; got != want {
		t.Fatalf("Content-Length: got %q, want %q", got, want)
	}
}

func TestPipeline_BodyReplace_ContentTypeFilterAndLiteralMode(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/text":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("a+b a+b"))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"msg":"a+b"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	host, port, err := net.SplitHostPort(upstream.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	rs := makeRuleSet(t, []config.RawRule{
		{
			Type:        "body_replace",
			Match:       config.RawMatch{Host: "^" + host + "$", Path: `^/`},
			Search:      "a+b",
			SearchMode:  "literal",
			Replace:     "z",
			ContentType: `^text/`,
		},
	}, t.TempDir())

	p := proxy.NewPipeline(rs, false)

	textResult, textErr := p.Run(&proxy.FlowContext{
		Logger: testLogger(), Scheme: "http", Host: host, Port: port,
		Method: "GET", Path: "/text", Header: make(http.Header),
	})
	if textErr != nil {
		t.Fatalf("Run /text: %v", textErr)
	}
	textBody, _ := io.ReadAll(textResult.Body)
	if got, want := string(textBody), "z z"; got != want {
		t.Fatalf("/text body: got %q, want %q", got, want)
	}

	jsonResult, jsonErr := p.Run(&proxy.FlowContext{
		Logger: testLogger(), Scheme: "http", Host: host, Port: port,
		Method: "GET", Path: "/json", Header: make(http.Header),
	})
	if jsonErr != nil {
		t.Fatalf("Run /json: %v", jsonErr)
	}
	jsonBody, _ := io.ReadAll(jsonResult.Body)
	if got, want := string(jsonBody), `{"msg":"a+b"}`; got != want {
		t.Fatalf("/json body: got %q, want %q", got, want)
	}
}

func TestPipeline_BodyReplace_AppliesToMapLocal(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()

	host, port, err := net.SplitHostPort(upstream.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	dir := t.TempDir()
	file := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(file, []byte("foo map"), 0o644); err != nil {
		t.Fatalf("write map_local file: %v", err)
	}

	rs := makeRuleSet(t, []config.RawRule{
		{
			Type:  "map_local",
			Match: config.RawMatch{Host: "^" + host + "$", Path: `^/local$`},
			Path:  file,
		},
		{
			Type:    "body_replace",
			Match:   config.RawMatch{Host: "^" + host + "$", Path: `^/`},
			Search:  "foo",
			Replace: "bar",
		},
	}, dir)

	p := proxy.NewPipeline(rs, false)

	localResult, localErr := p.Run(&proxy.FlowContext{
		Logger: testLogger(), Scheme: "http", Host: host, Port: port,
		Method: "GET", Path: "/local", Header: make(http.Header),
	})
	if localErr != nil {
		t.Fatalf("Run /local: %v", localErr)
	}
	localBody, _ := io.ReadAll(localResult.Body)
	if got, want := string(localBody), "bar map"; got != want {
		t.Fatalf("/local body: got %q, want %q", got, want)
	}
}

func TestPipeline_MapRemote_ReroutesUpstreamRequest(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("origin"))
	}))
	defer origin.Close()

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/mirror/v1/items"; got != want {
			t.Fatalf("path: got %q, want %q", got, want)
		}
		if got, want := r.URL.RawQuery, "env=dev&q=1"; got != want {
			t.Fatalf("raw query: got %q, want %q", got, want)
		}
		_, _ = w.Write([]byte("remote"))
	}))
	defer remote.Close()

	originHost, originPort, err := net.SplitHostPort(origin.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split origin host port: %v", err)
	}
	remoteHostPort := remote.Listener.Addr().String()

	rs := makeRuleSet(t, []config.RawRule{
		{
			Type:  "map_remote",
			Match: config.RawMatch{Host: "^" + originHost + "$", Path: `^/api/`},
			URL:   "http://" + remoteHostPort + "/mirror/?env=dev",
		},
	}, t.TempDir())

	p := proxy.NewPipeline(rs, false)
	result, runErr := p.Run(&proxy.FlowContext{
		Logger: testLogger(), Scheme: "http", Host: originHost, Port: originPort,
		Method: "GET", Path: "/api/v1/items", RawQuery: "q=1", Header: make(http.Header),
	})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("status: got %d, want %d", result.Status, http.StatusOK)
	}
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got, want := string(body), "remote"; got != want {
		t.Fatalf("body: got %q, want %q", got, want)
	}
}

func TestPipeline_MapLocal_PrecedesMapRemote(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("origin"))
	}))
	defer origin.Close()

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("remote"))
	}))
	defer remote.Close()

	originHost, originPort, err := net.SplitHostPort(origin.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split origin host port: %v", err)
	}
	remoteHostPort := remote.Listener.Addr().String()

	dir := t.TempDir()
	localFile := filepath.Join(dir, "local.txt")
	if err := os.WriteFile(localFile, []byte("local"), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}

	rs := makeRuleSet(t, []config.RawRule{
		{
			Type:  "map_local",
			Match: config.RawMatch{Host: "^" + originHost + "$", Path: `^/api/`},
			Path:  localFile,
		},
		{
			Type:  "map_remote",
			Match: config.RawMatch{Host: "^" + originHost + "$", Path: `^/api/`},
			URL:   "http://" + remoteHostPort + "/mirror/",
		},
	}, dir)

	p := proxy.NewPipeline(rs, false)
	result, runErr := p.Run(&proxy.FlowContext{
		Logger: testLogger(), Scheme: "http", Host: originHost, Port: originPort,
		Method: "GET", Path: "/api/v1/items", Header: make(http.Header),
	})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if got, want := result.Source, "local"; got != want {
		t.Fatalf("source: got %q, want %q", got, want)
	}
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got, want := string(body), "local"; got != want {
		t.Fatalf("body: got %q, want %q", got, want)
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
