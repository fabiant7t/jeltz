package proxy_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	ghttp2 "golang.org/x/net/http2"

	"github.com/fabiant7t/jeltz/internal/ca"
	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/proxy"
	"github.com/fabiant7t/jeltz/internal/rules"
)

// startTestProxy starts an in-process jeltz proxy and returns its address,
// the CA instance, and a cancel func.
func startTestProxy(t *testing.T, rawRules []config.RawRule, basePath string) (proxyAddr string, caInst *ca.CA) {
	t.Helper()

	dataDir := t.TempDir()
	caInst, err := ca.Load(dataDir)
	if err != nil {
		t.Fatalf("ca.Load: %v", err)
	}

	rs, err := rules.Compile(rawRules, basePath)
	if err != nil {
		t.Fatalf("rules.Compile: %v", err)
	}
	pipeline := proxy.NewPipeline(rs, false)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := proxy.New("127.0.0.1:0", logger, pipeline, caInst)

	// Start on ephemeral port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	proxyAddr = ln.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { ln.Close() })

	go func() {
		// Use the listener directly via a custom ListenAndServe.
		hs := &http.Server{Handler: srv}
		go func() { <-ctx.Done(); hs.Close() }()
		hs.Serve(ln) //nolint:errcheck
	}()

	return proxyAddr, caInst
}

// buildCAPool loads the CA cert from caInst and returns an x509.CertPool.
func buildCAPool(t *testing.T, caInst *ca.CA) *x509.CertPool {
	t.Helper()
	pem, err := os.ReadFile(caInst.CertPath())
	if err != nil {
		t.Fatalf("read CA cert: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatal("failed to add CA cert to pool")
	}
	return pool
}

// connectTunnel dials the proxy and sends a CONNECT request, returning the raw conn.
func connectTunnel(t *testing.T, proxyAddr, target string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}

	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		t.Fatalf("write CONNECT: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		t.Fatalf("read CONNECT response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		t.Fatalf("CONNECT status: %d", resp.StatusCode)
	}
	return conn
}

func TestMITM_H2_ConcurrentStreams(t *testing.T) {
	// Prepare static files.
	dir := t.TempDir()
	staticDir := filepath.Join(dir, "static")
	_ = os.MkdirAll(staticDir, 0o755)
	for i := range 5 {
		name := fmt.Sprintf("file%d.txt", i)
		_ = os.WriteFile(filepath.Join(staticDir, name), []byte("content-"+name), 0o644)
	}

	rawRules := []config.RawRule{
		// Header rule: add X-Test to all responses.
		{
			Type:  "header",
			Match: config.RawMatch{Host: `^example\.com$`, Path: `.*`},
			Response: &config.RawOps{
				Set: []config.RawSetOp{{Name: "X-Test", Mode: "replace", Value: "1"}},
			},
		},
		// Map-local rule for /static/.
		{
			Type:  "map_local",
			Match: config.RawMatch{Host: `^example\.com$`, Path: `^/static/`},
			Path:  staticDir,
		},
	}

	proxyAddr, caInst := startTestProxy(t, rawRules, dir)
	pool := buildCAPool(t, caInst)

	// B) Establish CONNECT tunnel.
	target := "example.com:443"
	rawConn := connectTunnel(t, proxyAddr, target)

	// C) TLS handshake offering h2.
	tlsConf := &tls.Config{
		ServerName: "example.com",
		RootCAs:    pool,
		NextProtos: []string{"h2"},
	}
	tlsConn := tls.Client(rawConn, tlsConf)
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("TLS handshake: %v", err)
	}

	// Assert ALPN negotiated h2.
	negotiated := tlsConn.ConnectionState().NegotiatedProtocol
	if negotiated != "h2" {
		t.Fatalf("expected ALPN h2, got %q", negotiated)
	}

	// D) Speak HTTP/2 over TLS.
	tr := &ghttp2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			return tlsConn, nil
		},
		TLSClientConfig: tlsConf,
	}
	client := &http.Client{Transport: tr}

	// Send 20 concurrent requests across different files (cycling).
	const N = 20
	type result struct {
		idx    int
		status int
		body   string
		xtest  string
		err    error
	}
	results := make([]result, N)
	var wg sync.WaitGroup
	for i := range N {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			fname := fmt.Sprintf("file%d.txt", idx%5)
			url := "https://example.com/static/" + fname
			req, _ := http.NewRequest("GET", url, nil)
			req.Host = "example.com"
			resp, err := client.Do(req)
			if err != nil {
				results[idx] = result{idx: idx, err: err}
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			results[idx] = result{
				idx:    idx,
				status: resp.StatusCode,
				body:   string(body),
				xtest:  resp.Header.Get("X-Test"),
			}
		}(i)
	}
	wg.Wait()

	// Validate all results.
	for _, r := range results {
		if r.err != nil {
			t.Errorf("request %d: %v", r.idx, r.err)
			continue
		}
		if r.status != http.StatusOK {
			t.Errorf("request %d: status %d", r.idx, r.status)
		}
		fname := fmt.Sprintf("file%d.txt", r.idx%5)
		expected := "content-" + fname
		if r.body != expected {
			t.Errorf("request %d: body %q, want %q", r.idx, r.body, expected)
		}
		if r.xtest != "1" {
			t.Errorf("request %d: X-Test %q, want 1", r.idx, r.xtest)
		}
	}
}

func TestMITM_H2_ALPN_Negotiated(t *testing.T) {
	proxyAddr, caInst := startTestProxy(t, nil, t.TempDir())
	pool := buildCAPool(t, caInst)

	rawConn := connectTunnel(t, proxyAddr, "example.com:443")

	tlsConf := &tls.Config{
		ServerName: "example.com",
		RootCAs:    pool,
		NextProtos: []string{"h2"},
	}
	tlsConn := tls.Client(rawConn, tlsConf)
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("TLS handshake: %v", err)
	}
	if tlsConn.ConnectionState().NegotiatedProtocol != "h2" {
		t.Errorf("expected h2, got %q", tlsConn.ConnectionState().NegotiatedProtocol)
	}
	tlsConn.Close()
}

func TestMITM_HTTP1Fallback(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0o644)

	rawRules := []config.RawRule{
		{
			Type:  "map_local",
			Match: config.RawMatch{Host: `^example\.com$`, Path: `^/hello`},
			Path:  filepath.Join(dir, "hello.txt"),
		},
	}
	proxyAddr, caInst := startTestProxy(t, rawRules, dir)
	pool := buildCAPool(t, caInst)

	rawConn := connectTunnel(t, proxyAddr, "example.com:443")

	// Offer only http/1.1 — jeltz must fall back.
	tlsConf := &tls.Config{
		ServerName: "example.com",
		RootCAs:    pool,
		NextProtos: []string{"http/1.1"},
	}
	tlsConn := tls.Client(rawConn, tlsConf)
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("TLS handshake: %v", err)
	}
	defer tlsConn.Close()

	// HTTP/1.1 request over TLS.
	fmt.Fprintf(tlsConn, "GET /hello HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n")

	br := bufio.NewReader(tlsConn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hi" {
		t.Errorf("body: %q", body)
	}
}

func TestMITM_UpstreamPassthrough(t *testing.T) {
	// Start a local upstream HTTPS server.
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-From-Upstream", "yes")
		fmt.Fprint(w, "upstream-content")
	}))
	defer upstream.Close()

	upstreamHost := upstream.Listener.Addr().(*net.TCPAddr).IP.String()
	upstreamPort := fmt.Sprintf("%d", upstream.Listener.Addr().(*net.TCPAddr).Port)
	hostWithPort := upstreamHost + ":" + upstreamPort

	// Rule: add X-Intercepted to response.
	rawRules := []config.RawRule{
		{
			Type:  "header",
			Match: config.RawMatch{Host: `.*`, Path: `.*`},
			Response: &config.RawOps{
				Set: []config.RawSetOp{{Name: "X-Intercepted", Mode: "replace", Value: "true"}},
			},
		},
	}

	// Use insecure_upstream=true to reach the test TLS server.
	dataDir := t.TempDir()
	caInst, _ := ca.Load(dataDir)
	rs, _ := rules.Compile(rawRules, t.TempDir())
	pipeline := proxy.NewPipeline(rs, true /* insecure upstream */)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := proxy.New("127.0.0.1:0", logger, pipeline, caInst)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	proxyAddr := ln.Addr().String()
	ctx := t.Context()
	go func() {
		hs := &http.Server{Handler: srv}
		go func() { <-ctx.Done(); hs.Close() }()
		hs.Serve(ln) //nolint:errcheck
	}()

	pool := buildCAPool(t, caInst)
	rawConn := connectTunnel(t, proxyAddr, hostWithPort)

	tlsConf := &tls.Config{
		ServerName: upstreamHost,
		RootCAs:    pool,
		NextProtos: []string{"http/1.1"}, // use fallback path for this test
	}
	tlsConn := tls.Client(rawConn, tlsConf)
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("TLS handshake: %v", err)
	}
	defer tlsConn.Close()

	proto := tlsConn.ConnectionState().NegotiatedProtocol
	_ = proto

	// Use HTTP/1.1 for simplicity in this test.
	fmt.Fprintf(tlsConn, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", upstreamHost)
	br := bufio.NewReader(tlsConn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Intercepted") != "true" {
		t.Errorf("X-Intercepted: got %q", resp.Header.Get("X-Intercepted"))
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "upstream-content") {
		t.Errorf("body: %q", body)
	}
}
