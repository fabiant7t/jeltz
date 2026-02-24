package proxy

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	ghttp2 "golang.org/x/net/http2"

	"github.com/fabiant7t/jeltz/internal/ca"
	"github.com/fabiant7t/jeltz/internal/logging"
)

// mitmHandler handles a CONNECT tunnel with full TLS MITM and HTTP/2 support.
// It is called by ServeHTTP after hijacking the connection.
func (s *Server) mitmHandler(clientConn net.Conn, targetHost, targetPort, clientAddr string) {
	defer clientConn.Close()

	// Signal tunnel established.
	_, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		s.logger.Error("mitm write 200",
			slog.String(logging.KeyComponent, "mitm"),
			slog.String(logging.KeyEvent, "mitm_handshake_error"),
			slog.String(logging.KeyError, err.Error()),
		)
		return
	}

	// Wrap in TLS, offering ALPN h2 and http/1.1.
	tlsConf := &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			host := hello.ServerName
			if host == "" {
				host = targetHost
			}
			return s.ca.LeafCert(host)
		},
		NextProtos: []string{"h2", "http/1.1"},
	}
	tlsConn := tls.Server(clientConn, tlsConf)
	if err := tlsConn.Handshake(); err != nil {
		s.logger.Error("mitm TLS handshake",
			slog.String(logging.KeyComponent, "mitm"),
			slog.String(logging.KeyEvent, "mitm_handshake_error"),
			slog.String(logging.KeyHost, targetHost),
			slog.String(logging.KeyError, err.Error()),
		)
		return
	}

	proto := tlsConn.ConnectionState().NegotiatedProtocol
	s.logger.Debug("mitm TLS established",
		slog.String(logging.KeyComponent, "mitm"),
		slog.String(logging.KeyHost, targetHost),
		slog.String(logging.KeyProto, proto),
	)

	switch proto {
	case "h2":
		s.serveH2(tlsConn, targetHost, targetPort, clientAddr)
	default:
		if proto != "" && proto != "http/1.1" {
			s.logger.Warn("mitm: unexpected ALPN proto, falling back to HTTP/1.1",
				slog.String(logging.KeyComponent, "mitm"),
				slog.String(logging.KeyProto, proto),
			)
		}
		s.serveHTTP1(tlsConn, targetHost, targetPort, clientAddr)
	}
}

// serveH2 serves HTTP/2 over the hijacked TLS connection.
func (s *Server) serveH2(tlsConn *tls.Conn, targetHost, targetPort, clientAddr string) {
	h2Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		fc := &FlowContext{
			Logger:     s.logger,
			ClientAddr: clientAddr,
			Proto:      "h2",
			Scheme:     "https",
			Host:       targetHost,
			Port:       targetPort,
			Method:     r.Method,
			Path:       r.URL.Path,
			RawQuery:   r.URL.RawQuery,
			Header:     r.Header.Clone(),
			Body:       r.Body,
			Ctx:        r.Context(), // propagates per-stream cancellation
		}

		result, err := s.pipeline.Run(fc)
		if err != nil {
			s.logger.Error("h2 pipeline error",
				slog.String(logging.KeyComponent, "mitm"),
				slog.String(logging.KeyEvent, "h2_serve_error"),
				slog.String(logging.KeyError, err.Error()),
			)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		for k, vals := range result.Headers {
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(result.Status)
		if result.Body != nil {
			io.Copy(w, result.Body) //nolint:errcheck
			result.Body.Close()    //nolint:errcheck
		}

		s.logger.Info("request",
			slog.String(logging.KeyComponent, "mitm"),
			slog.String(logging.KeyClient, clientAddr),
			slog.String(logging.KeyMethod, r.Method),
			slog.String(logging.KeyScheme, "https"),
			slog.String(logging.KeyHost, targetHost),
			slog.String(logging.KeyPath, r.URL.Path),
			slog.Int(logging.KeyStatus, result.Status),
			slog.String(logging.KeySource, result.Source),
			slog.Int64(logging.KeyDurationMS, time.Since(start).Milliseconds()),
			slog.String(logging.KeyProto, "h2"),
		)
	})

	var h2s ghttp2.Server
	h2s.ServeConn(tlsConn, &ghttp2.ServeConnOpts{
		Handler: h2Handler,
	})
}

// serveHTTP1 serves HTTP/1.1 requests over the hijacked TLS connection in a loop.
func (s *Server) serveHTTP1(tlsConn *tls.Conn, targetHost, targetPort, clientAddr string) {
	br := bufio.NewReader(tlsConn)
	for {
		req, err := http.ReadRequest(br)
		if err != nil {
			if err != io.EOF {
				s.logger.Debug("mitm HTTP/1.1 read",
					slog.String(logging.KeyComponent, "mitm"),
					slog.String(logging.KeyEvent, "upstream_error"),
					slog.String(logging.KeyError, err.Error()),
				)
			}
			return
		}

		start := time.Now()
		fc := &FlowContext{
			Logger:     s.logger,
			ClientAddr: clientAddr,
			Proto:      "http/1.1",
			Scheme:     "https",
			Host:       targetHost,
			Port:       targetPort,
			Method:     req.Method,
			Path:       req.URL.Path,
			RawQuery:   req.URL.RawQuery,
			Header:     req.Header.Clone(),
			Body:       req.Body,
		}

		result, err := s.pipeline.Run(fc)
		if err != nil {
			s.logger.Error("HTTP/1.1 pipeline error",
				slog.String(logging.KeyComponent, "mitm"),
				slog.String(logging.KeyEvent, "upstream_error"),
				slog.String(logging.KeyError, err.Error()),
			)
			writeHTTP1Error(tlsConn, http.StatusInternalServerError)
			return
		}

		if err := writeHTTP1Response(tlsConn, result); err != nil {
			return
		}

		s.logger.Info("request",
			slog.String(logging.KeyComponent, "mitm"),
			slog.String(logging.KeyClient, clientAddr),
			slog.String(logging.KeyMethod, req.Method),
			slog.String(logging.KeyScheme, "https"),
			slog.String(logging.KeyHost, targetHost),
			slog.String(logging.KeyPath, req.URL.Path),
			slog.Int(logging.KeyStatus, result.Status),
			slog.String(logging.KeySource, result.Source),
			slog.Int64(logging.KeyDurationMS, time.Since(start).Milliseconds()),
			slog.String(logging.KeyProto, "http/1.1"),
		)

		if req.Close {
			return
		}
	}
}

func writeHTTP1Response(conn net.Conn, result *ResponseResult) error {
	resp := &http.Response{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		StatusCode: result.Status,
		Status:     fmt.Sprintf("%d %s", result.Status, http.StatusText(result.Status)),
		Header:     result.Headers,
		Body:       result.Body,
	}
	if result.Body == nil {
		resp.Body = http.NoBody
	}
	defer func() {
		if result.Body != nil {
			result.Body.Close() //nolint:errcheck
		}
	}()
	return resp.Write(conn)
}

func writeHTTP1Error(conn net.Conn, code int) {
	msg := fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Length: 0\r\n\r\n", code, http.StatusText(code))
	conn.Write([]byte(msg)) //nolint:errcheck
}

// targetHostPort parses "host:port" from a CONNECT request's Host field.
// Returns host (no port) and port string.
func targetHostPort(hostPort string) (host, port string) {
	if h, p, err := net.SplitHostPort(hostPort); err == nil {
		return h, p
	}
	// No port — strip any colon.
	return strings.TrimSuffix(hostPort, ":"), ""
}

// caLoader is satisfied by *ca.CA; allows testing with mocks.
type caLoader interface {
	LeafCert(host string) (*tls.Certificate, error)
}

// Ensure *ca.CA satisfies caLoader.
var _ caLoader = (*ca.CA)(nil)
