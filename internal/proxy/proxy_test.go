package proxy_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fabiant7t/jeltz/internal/proxy"
)

func startRawProxy(t *testing.T) string {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := proxy.New("127.0.0.1:0", logger, nil, nil)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() {
		hs := &http.Server{Handler: srv}
		go func() { <-ctx.Done(); _ = hs.Close() }()
		_ = hs.Serve(ln)
	}()

	return ln.Addr().String()
}

func sendCONNECT(t *testing.T, proxyAddr, target string) (net.Conn, *http.Response) {
	t.Helper()

	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}

	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		t.Fatalf("write CONNECT: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read CONNECT response: %v", err)
	}
	return conn, resp
}

func TestHandleForward_RejectsInvalidURL(t *testing.T) {
	srv := proxy.New("127.0.0.1:0", testLogger(), nil, nil)

	t.Run("nil URL", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		r.URL = nil

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("non-absolute URL", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
		r.URL = &url.URL{Path: "/only-path"}

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d, want %d", w.Code, http.StatusBadRequest)
		}
		if !strings.Contains(w.Body.String(), "absolute URL required") {
			t.Fatalf("body: got %q", w.Body.String())
		}
	})
}

func TestHandleForward_NoPipeline_ForwardsAndStripsHopHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Saw-Connection", r.Header.Get("Connection"))
		w.Header().Set("X-Saw-Proxy-Authorization", r.Header.Get("Proxy-Authorization"))
		w.Header().Set("Proxy-Authenticate", "Basic realm=test")
		w.Header().Set("X-Upstream", "yes")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("forward-ok"))
	}))
	defer upstream.Close()

	srv := proxy.New("127.0.0.1:0", testLogger(), nil, nil)
	r := httptest.NewRequest(http.MethodGet, upstream.URL+"/x", nil)
	r.Header.Set("Connection", "keep-alive")
	r.Header.Set("Proxy-Authorization", "Basic abc")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Header().Get("X-Saw-Connection") != "" {
		t.Fatalf("upstream got Connection header %q; expected stripped", w.Header().Get("X-Saw-Connection"))
	}
	if w.Header().Get("X-Saw-Proxy-Authorization") != "" {
		t.Fatalf("upstream got Proxy-Authorization header %q; expected stripped", w.Header().Get("X-Saw-Proxy-Authorization"))
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusCreated)
	}
	if w.Body.String() != "forward-ok" {
		t.Fatalf("body: got %q", w.Body.String())
	}
	if w.Header().Get("X-Upstream") != "yes" {
		t.Fatalf("X-Upstream: got %q", w.Header().Get("X-Upstream"))
	}
	if w.Header().Get("Proxy-Authenticate") != "" {
		t.Fatalf("Proxy-Authenticate should be stripped, got %q", w.Header().Get("Proxy-Authenticate"))
	}
}

func TestHandleForward_NoPipeline_UpstreamErrorReturns502(t *testing.T) {
	srv := proxy.New("127.0.0.1:0", testLogger(), nil, nil)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	target := ln.Addr().String()
	_ = ln.Close()

	r := httptest.NewRequest(http.MethodGet, "http://"+target+"/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusBadGateway)
	}
}

func TestHandleForward_PipelinePath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pipeline-ok"))
	}))
	defer upstream.Close()

	rs := makeRuleSet(t, nil, t.TempDir())
	p := proxy.NewPipeline(rs, false)
	srv := proxy.New("127.0.0.1:0", testLogger(), p, nil)

	r := httptest.NewRequest(http.MethodGet, upstream.URL+"/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "pipeline-ok" {
		t.Fatalf("body: got %q", w.Body.String())
	}
}

func TestHandleForward_PipelineErrorReturns500(t *testing.T) {
	rs := makeRuleSet(t, nil, t.TempDir())
	p := proxy.NewPipeline(rs, false)
	srv := proxy.New("127.0.0.1:0", testLogger(), p, nil)

	r := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	r.Method = "BAD METHOD"

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleForward_NoPipeline_PreservesQueryString(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Got-Query", r.URL.RawQuery)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	srv := proxy.New("127.0.0.1:0", testLogger(), nil, nil)
	r := httptest.NewRequest(http.MethodGet, upstream.URL+"/search?q=test&lang=en", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	if w.Header().Get("X-Got-Query") != "q=test&lang=en" {
		t.Fatalf("query: got %q", w.Header().Get("X-Got-Query"))
	}
}

func TestHandleForward_NoPipeline_ForwardsRequestBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer upstream.Close()

	srv := proxy.New("127.0.0.1:0", testLogger(), nil, nil)
	payload := "hello-body"
	r := httptest.NewRequest(http.MethodPost, upstream.URL+"/submit", strings.NewReader(payload))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != payload {
		t.Fatalf("body: got %q, want %q", w.Body.String(), payload)
	}
}

func TestRawTunnel_UnreachableTargetReturns502(t *testing.T) {
	proxyAddr := startRawProxy(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	target := ln.Addr().String()
	_ = ln.Close()

	conn, resp := sendCONNECT(t, proxyAddr, target)
	defer conn.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
}

func TestRawTunnel_BidirectionalCopy(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen upstream: %v", err)
	}
	defer upstream.Close()

	go func() {
		conn, err := upstream.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 256)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		_, _ = conn.Write(buf[:n])
	}()

	proxyAddr := startRawProxy(t)
	conn, resp := sendCONNECT(t, proxyAddr, upstream.Addr().String())
	defer conn.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if d, ok := t.Deadline(); ok {
		_ = conn.SetDeadline(d)
	} else {
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	}
	payload := []byte("hello over tunnel")
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write tunnel payload: %v", err)
	}

	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read echoed payload: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("echoed payload: got %q, want %q", string(got), string(payload))
	}
}

func TestRawTunnel_ClientCloseWrite(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen upstream: %v", err)
	}
	defer upstream.Close()

	done := make(chan int, 1)
	go func() {
		conn, err := upstream.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		got, err := io.ReadAll(conn)
		if err != nil {
			return
		}
		done <- len(got)
	}()

	proxyAddr := startRawProxy(t)
	conn, resp := sendCONNECT(t, proxyAddr, upstream.Addr().String())
	defer conn.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		t.Fatalf("expected *net.TCPConn, got %T", conn)
	}

	payload := []byte("abc123")
	if _, err := tcpConn.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := tcpConn.CloseWrite(); err != nil {
		t.Fatalf("close write: %v", err)
	}

	select {
	case n := <-done:
		if n != len(payload) {
			t.Fatalf("upstream read %d bytes, want %d", n, len(payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for upstream EOF completion")
	}

	if d, ok := t.Deadline(); ok {
		_ = tcpConn.SetDeadline(d)
	} else {
		_ = tcpConn.SetDeadline(time.Now().Add(2 * time.Second))
	}
	b := make([]byte, 1)
	_, err = tcpConn.Read(b)
	if err == nil {
		t.Fatal("expected EOF after close-write path")
	}
}

func TestRawTunnel_LargePayload(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen upstream: %v", err)
	}
	defer upstream.Close()

	go func() {
		conn, err := upstream.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 32*1024)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				if _, werr := conn.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	proxyAddr := startRawProxy(t)
	conn, resp := sendCONNECT(t, proxyAddr, upstream.Addr().String())
	defer conn.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if d, ok := t.Deadline(); ok {
		_ = conn.SetDeadline(d)
	} else {
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	}

	payload := bytes.Repeat([]byte("0123456789abcdef"), 64*1024) // 1 MiB
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write large payload: %v", err)
	}

	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read large payload: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("large payload mismatch")
	}
}
