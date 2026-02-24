// Package proxy implements the jeltz HTTP proxy server.
package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/fabiant7t/jeltz/internal/httpx"
	"github.com/fabiant7t/jeltz/internal/logging"
)

// Server is the jeltz proxy server.
type Server struct {
	listen string
	logger *slog.Logger
	// pipeline will be wired in L6+
}

// New creates a new proxy Server.
func New(listen string, logger *slog.Logger) *Server {
	return &Server{
		listen: listen,
		logger: logger,
	}
}

// ListenAndServe starts the proxy server and blocks until the context is done.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.listen,
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	s.logger.Info("proxy listening",
		slog.String(logging.KeyComponent, "proxy"),
		slog.String("addr", s.listen),
	)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("proxy: %w", err)
	}
	return nil
}

// ServeHTTP dispatches incoming requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		s.handleCONNECT(w, r)
	} else {
		s.handleForward(w, r)
	}
}

// handleCONNECT is a raw TCP tunnel stub (replaced in L8 with MITM).
func (s *Server) handleCONNECT(w http.ResponseWriter, r *http.Request) {
	targetAddr := r.Host
	s.logger.Debug("CONNECT (tunnel stub)",
		slog.String(logging.KeyComponent, "proxy"),
		slog.String(logging.KeyClient, r.RemoteAddr),
		slog.String(logging.KeyHost, targetAddr),
	)

	upstream, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	hij, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hij.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	done := make(chan struct{}, 2)
	go func() { io.Copy(upstream, clientConn); done <- struct{}{} }()   //nolint:errcheck
	go func() { io.Copy(clientConn, upstream); done <- struct{}{} }()   //nolint:errcheck
	<-done
}

// handleForward handles non-CONNECT (plain HTTP) forward proxy requests.
func (s *Server) handleForward(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.URL == nil || !r.URL.IsAbs() {
		http.Error(w, "Bad Request: absolute URL required", http.StatusBadRequest)
		return
	}

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	outReq.Header = r.Header.Clone()
	httpx.RemoveHopByHop(outReq.Header)

	tr := &http.Transport{}
	resp, err := tr.RoundTrip(outReq)
	if err != nil {
		s.logger.Error("upstream error",
			slog.String(logging.KeyComponent, "proxy"),
			slog.String(logging.KeyEvent, "upstream_error"),
			slog.String(logging.KeyClient, r.RemoteAddr),
			slog.String(logging.KeyMethod, r.Method),
			slog.String(logging.KeyHost, r.URL.Host),
			slog.String(logging.KeyPath, r.URL.Path),
			slog.String(logging.KeyError, err.Error()),
		)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	httpx.RemoveHopByHop(resp.Header)
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck

	s.logger.Info("forward",
		slog.String(logging.KeyComponent, "proxy"),
		slog.String(logging.KeyClient, r.RemoteAddr),
		slog.String(logging.KeyMethod, r.Method),
		slog.String(logging.KeyScheme, r.URL.Scheme),
		slog.String(logging.KeyHost, r.URL.Host),
		slog.String(logging.KeyPath, r.URL.Path),
		slog.Int(logging.KeyStatus, resp.StatusCode),
		slog.String(logging.KeySource, "upstream"),
		slog.Int64(logging.KeyDurationMS, time.Since(start).Milliseconds()),
		slog.String(logging.KeyProto, "http/1.1"),
	)
}
