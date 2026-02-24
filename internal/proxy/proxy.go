// Package proxy implements the jeltz HTTP proxy server.
package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/fabiant7t/jeltz/internal/httpx"
	"github.com/fabiant7t/jeltz/internal/logging"
)

// Server is the jeltz proxy server.
type Server struct {
	listen   string
	logger   *slog.Logger
	pipeline *Pipeline
	ca       caLoader
}

// New creates a new proxy Server.
func New(listen string, logger *slog.Logger, pipeline *Pipeline, ca caLoader) *Server {
	return &Server{
		listen:   listen,
		logger:   logger,
		pipeline: pipeline,
		ca:       ca,
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

// handleCONNECT performs TLS MITM when CA is configured, else raw tunnel.
func (s *Server) handleCONNECT(w http.ResponseWriter, r *http.Request) {
	targetAddr := r.Host
	host, port := targetHostPort(targetAddr)

	s.logger.Debug("CONNECT",
		slog.String(logging.KeyComponent, "proxy"),
		slog.String(logging.KeyClient, r.RemoteAddr),
		slog.String(logging.KeyHost, host),
		slog.String("port", port),
	)

	hij, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hij.Hijack()
	if err != nil {
		return
	}

	if s.ca != nil && s.pipeline != nil {
		// Full MITM with TLS interception (L8).
		go s.mitmHandler(clientConn, host, port, r.RemoteAddr)
		return
	}

	// Fallback: raw TCP tunnel (no CA configured).
	rawTunnel(clientConn, targetAddr, s.logger)
}

// rawTunnel dials target and bidirectionally copies between clientConn and it.
// When either direction finishes, the corresponding connection is closed to
// unblock the other goroutine. Both goroutines are awaited before returning.
func rawTunnel(clientConn net.Conn, targetAddr string, logger *slog.Logger) {
	defer clientConn.Close()

	upstream, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		logger.Error("tunnel dial",
			slog.String(logging.KeyComponent, "proxy"),
			slog.String(logging.KeyEvent, "upstream_error"),
			slog.String(logging.KeyError, err.Error()),
		)
		writeHTTP1Error(clientConn, http.StatusBadGateway)
		return
	}

	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		upstream.Close()
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(upstream, clientConn) //nolint:errcheck
		upstream.Close()
	}()
	go func() {
		defer wg.Done()
		io.Copy(clientConn, upstream) //nolint:errcheck
		clientConn.Close()
	}()
	wg.Wait()
}

// handleForward handles non-CONNECT (plain HTTP) forward proxy requests.
func (s *Server) handleForward(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	if r.URL == nil || !r.URL.IsAbs() {
		http.Error(w, "Bad Request: absolute URL required", http.StatusBadRequest)
		return
	}

	if s.pipeline != nil {
		host, port := targetHostPort(r.URL.Host)
		scheme := r.URL.Scheme
		if scheme == "" {
			scheme = "http"
		}
		fc := &FlowContext{
			Logger:     s.logger,
			ClientAddr: r.RemoteAddr,
			Proto:      "http/1.1",
			Scheme:     scheme,
			Host:       host,
			Port:       port,
			Method:     r.Method,
			Path:       r.URL.Path,
			RawQuery:   r.URL.RawQuery,
			Header:     r.Header.Clone(),
			Body:       r.Body,
			Ctx:        r.Context(),
		}
		result, err := s.pipeline.Run(fc)
		if err != nil {
			s.logger.Error("pipeline error",
				slog.String(logging.KeyComponent, "proxy"),
				slog.String(logging.KeyEvent, "upstream_error"),
				slog.String(logging.KeyError, err.Error()),
			)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		WriteResponse(w, result, fc, start)
		return
	}

	// Fallback: direct forward (pipeline not configured).
	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	outReq.Header = r.Header.Clone()
	httpx.RemoveHopByHop(outReq.Header)

	resp, err := http.DefaultTransport.RoundTrip(outReq)
	if err != nil {
		s.logger.Error("upstream error",
			slog.String(logging.KeyComponent, "proxy"),
			slog.String(logging.KeyEvent, "upstream_error"),
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
