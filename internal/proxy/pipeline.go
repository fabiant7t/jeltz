package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fabiant7t/jeltz/internal/httpx"
	"github.com/fabiant7t/jeltz/internal/logging"
	"github.com/fabiant7t/jeltz/internal/rules"
)

// FlowContext carries the metadata and mutable state for one proxied request.
type FlowContext struct {
	Logger     *slog.Logger
	ClientAddr string
	Proto      string // "http/1.1" or "h2"
	Scheme     string // "http" or "https"
	Host       string // hostname only, without port
	Port       string
	Method     string
	Path       string
	RawQuery   string
	Header     http.Header     // mutable request headers
	Body       io.ReadCloser   // may be nil
	Ctx        context.Context // per-request context for cancellation
}

// FlowMeta returns a rules.FlowMeta derived from this context.
func (fc *FlowContext) FlowMeta() rules.FlowMeta {
	fullPath := fc.Path
	if fc.RawQuery != "" {
		fullPath = fc.Path + "?" + fc.RawQuery
	}
	return rules.FlowMeta{
		Method:            fc.Method,
		Scheme:            fc.Scheme,
		Host:              fc.Host,
		Port:              fc.Port,
		Path:              fc.Path,
		RawQuery:          fc.RawQuery,
		FullPathWithQuery: fullPath,
	}
}

// ResponseResult is the outcome of running the pipeline.
type ResponseResult struct {
	Status  int
	Headers http.Header
	Body    io.ReadCloser
	Source  string // "local" or "upstream"
}

// Pipeline executes the full request/response processing chain.
type Pipeline struct {
	ruleset                     *rules.RuleSet
	transport                   *http.Transport // shared; created once
	dumpTraffic                 bool
	maxBodyBytes                int64
	maxUpstreamRequestBodyBytes int64
}

// TransportTimeouts configures upstream transport timeouts.
type TransportTimeouts struct {
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
}

// DefaultTransportTimeouts returns the upstream transport timeout defaults.
func DefaultTransportTimeouts() TransportTimeouts {
	return TransportTimeouts{
		DialTimeout:           10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       60 * time.Second,
	}
}

// NewPipeline creates a Pipeline.
func NewPipeline(rs *rules.RuleSet, insecureUpstream bool) *Pipeline {
	timeouts := DefaultTransportTimeouts()
	return &Pipeline{
		ruleset: rs,
		transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: timeouts.DialTimeout,
			}).DialContext,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureUpstream, //nolint:gosec
			},
			TLSHandshakeTimeout:   timeouts.TLSHandshakeTimeout,
			ResponseHeaderTimeout: timeouts.ResponseHeaderTimeout,
			IdleConnTimeout:       timeouts.IdleConnTimeout,
			Proxy:                 nil,
		},
	}
}

// WithDumpTraffic enables traffic dumping with the given body byte limit.
func (p *Pipeline) WithDumpTraffic(maxBodyBytes int64) *Pipeline {
	p.dumpTraffic = true
	p.maxBodyBytes = maxBodyBytes
	return p
}

// WithMaxUpstreamRequestBodyBytes sets the maximum allowed upstream request
// body size in bytes. Zero or negative means unlimited.
func (p *Pipeline) WithMaxUpstreamRequestBodyBytes(maxBytes int64) *Pipeline {
	p.maxUpstreamRequestBodyBytes = maxBytes
	return p
}

// WithTransportTimeouts sets upstream transport timeout values.
func (p *Pipeline) WithTransportTimeouts(timeouts TransportTimeouts) *Pipeline {
	p.transport.DialContext = (&net.Dialer{Timeout: timeouts.DialTimeout}).DialContext
	p.transport.TLSHandshakeTimeout = timeouts.TLSHandshakeTimeout
	p.transport.ResponseHeaderTimeout = timeouts.ResponseHeaderTimeout
	p.transport.IdleConnTimeout = timeouts.IdleConnTimeout
	return p
}

// Run processes fc and returns a ResponseResult.
func (p *Pipeline) Run(fc *FlowContext) (*ResponseResult, error) {
	fm := fc.FlowMeta()

	// Step 2: apply matching request header rules.
	if p.ruleset != nil {
		for _, hr := range p.ruleset.Headers {
			if hr.Match.Matches(fm) {
				hr.Request.Apply(fc.Header)
			}
		}
	}

	// Dump request headers after transforms.
	if p.dumpTraffic {
		dumpHeaders(fc.Logger, "request", fc.Header)
	}

	// Step 3: choose request routing — map/map-local first-match in file order,
	// else optional map-remote destination remap, then upstream.
	var result *ResponseResult
	var mapResponseOps *rules.Ops
	var mapRemoteTarget *rules.MapRemoteTarget

	if p.ruleset != nil {
	mapLoop:
		for _, mapped := range p.ruleset.Mapped {
			if mapped.MapLocal != nil {
				mlResult, err := mapped.MapLocal.Resolve(fm)
				if err != nil {
					if rules.IsTraversal(err) {
						return emptyResult(http.StatusForbidden, "local"), nil
					}
					return nil, fmt.Errorf("map-local resolve: %w", err)
				}
				if mlResult != nil {
					r, err := serveLocal(mlResult)
					if err != nil {
						return nil, err
					}
					mapResponseOps = mlResult.Response
					result = r
					break mapLoop
				}
			}
			if mapped.Map != nil {
				mResult, err := mapped.Map.Resolve(fm)
				if err != nil {
					return nil, fmt.Errorf("map resolve: %w", err)
				}
				if mResult != nil {
					result = serveMapped(mResult)
					mapResponseOps = mResult.Response
					break mapLoop
				}
			}
		}

		if result == nil {
			for _, mr := range p.ruleset.MapRemote {
				target, err := mr.Resolve(fm)
				if err != nil {
					return nil, fmt.Errorf("map-remote resolve: %w", err)
				}
				if target != nil {
					mapRemoteTarget = target
					break
				}
			}
		}
	}

	if result == nil {
		var err error
		result, err = p.roundtrip(fc, mapRemoteTarget)
		if err != nil {
			return nil, err
		}
	}

	// Step 4: apply matching body_replace rules in file order.
	if p.ruleset != nil && len(p.ruleset.BodyReplace) > 0 && result.Body != nil &&
		isIdentityContentEncoding(result.Headers.Values("Content-Encoding")) {
		contentType := result.Headers.Get("Content-Type")
		var matched []*rules.BodyReplaceRule
		for _, br := range p.ruleset.BodyReplace {
			if br.Matches(fm, contentType) {
				matched = append(matched, br)
			}
		}
		if len(matched) > 0 {
			bodyData, err := io.ReadAll(result.Body)
			result.Body.Close() //nolint:errcheck
			if err != nil {
				return nil, fmt.Errorf("reading response body for body_replace: %w", err)
			}
			for _, br := range matched {
				bodyData = br.Apply(bodyData)
			}
			result.Body = io.NopCloser(bytes.NewReader(bodyData))
			result.Headers.Del("Content-Length")
			result.Headers.Set("Content-Length", strconv.Itoa(len(bodyData)))
		}
	}

	// Step 5: apply matching response header rules.
	if p.ruleset != nil {
		for _, hr := range p.ruleset.Headers {
			if hr.Match.Matches(fm) {
				hr.Response.Apply(result.Headers)
			}
		}
	}

	// Step 6: apply map/map-local response ops after global response rules.
	if mapResponseOps != nil {
		mapResponseOps.Apply(result.Headers)
	}

	// Dump response headers after all transforms.
	if p.dumpTraffic {
		dumpHeaders(fc.Logger, "response", result.Headers)
		result.Body = dumpBody(fc.Logger, result.Body, p.maxBodyBytes)
	}

	return result, nil
}

func isIdentityContentEncoding(values []string) bool {
	if len(values) == 0 {
		return true
	}
	for _, v := range values {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			token := strings.TrimSpace(p)
			if token == "" || strings.EqualFold(token, "identity") {
				continue
			}
			return false
		}
	}
	return true
}

func emptyResult(status int, source string) *ResponseResult {
	return &ResponseResult{
		Status:  status,
		Headers: make(http.Header),
		Body:    io.NopCloser(bytes.NewReader(nil)),
		Source:  source,
	}
}

// serveMapped builds a ResponseResult from an inline map rule result.
func serveMapped(mr *rules.MapResult) *ResponseResult {
	ct := mr.ContentType
	if ct == "" {
		ct = http.DetectContentType(mr.Body)
	}
	h := make(http.Header)
	h.Set("Content-Type", ct)
	h.Set("Content-Length", strconv.Itoa(len(mr.Body)))

	return &ResponseResult{
		Status:  mr.StatusCode,
		Headers: h,
		Body:    io.NopCloser(bytes.NewReader(mr.Body)),
		Source:  "local",
	}
}

// serveLocal builds a ResponseResult from a MapLocalResult.
func serveLocal(mlr *rules.MapLocalResult) (*ResponseResult, error) {
	f, err := os.Open(mlr.FSTarget)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyResult(http.StatusNotFound, "local"), nil
		}
		return nil, fmt.Errorf("reading local file %q: %w", mlr.FSTarget, err)
	}
	// Keep file open for streaming as response body.
	// It is closed by WriteResponse after io.Copy.

	snip := make([]byte, 512)
	n, readErr := io.ReadFull(f, snip)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		f.Close() //nolint:errcheck
		return nil, fmt.Errorf("reading local file %q: %w", mlr.FSTarget, readErr)
	}
	snip = snip[:n]
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close() //nolint:errcheck
		return nil, fmt.Errorf("seeking local file %q: %w", mlr.FSTarget, err)
	}

	ct := rules.DetectContentType(mlr.FSTarget, mlr.ContentType, func(_ string) ([]byte, error) {
		return snip, nil
	})

	h := make(http.Header)
	h.Set("Content-Type", ct)

	return &ResponseResult{
		Status:  mlr.StatusCode,
		Headers: h,
		Body:    f,
		Source:  "local",
	}, nil
}

// roundtrip performs an upstream HTTP request using fc's context and the
// shared transport (connection pooling).
func (p *Pipeline) roundtrip(fc *FlowContext, mapRemoteTarget *rules.MapRemoteTarget) (*ResponseResult, error) {
	scheme := fc.Scheme
	host := fc.Host
	port := fc.Port
	path := fc.Path
	rawQuery := fc.RawQuery

	if mapRemoteTarget != nil {
		if mapRemoteTarget.Scheme != "" {
			scheme = mapRemoteTarget.Scheme
		}
		if mapRemoteTarget.Host != "" {
			host = mapRemoteTarget.Host
		}
		port = mapRemoteTarget.Port
		if mapRemoteTarget.Path != "" {
			path = mapRemoteTarget.Path
		}
		rawQuery = mapRemoteTarget.RawQuery
	}

	targetHost := host
	if port != "" {
		targetHost = host + ":" + port
	}

	targetURL := scheme + "://" + targetHost + path
	if rawQuery != "" {
		targetURL += "?" + rawQuery
	}

	ctx := fc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	var body io.Reader
	if fc.Body != nil {
		if p.maxUpstreamRequestBodyBytes > 0 {
			data, tooLarge, readErr := readRequestBody(fc.Body, p.maxUpstreamRequestBodyBytes)
			if readErr != nil {
				return nil, fmt.Errorf("reading request body: %w", readErr)
			}
			if tooLarge {
				return emptyResult(http.StatusRequestEntityTooLarge, "local"), nil
			}
			body = bytes.NewReader(data)
		} else {
			body = fc.Body
		}
	}
	outReq, err := http.NewRequestWithContext(ctx, fc.Method, targetURL, body)
	if err != nil {
		return nil, fmt.Errorf("building upstream request: %w", err)
	}
	for k, vals := range fc.Header {
		for _, v := range vals {
			outReq.Header.Add(k, v)
		}
	}
	httpx.RemoveHopByHop(outReq.Header)

	resp, err := p.transport.RoundTrip(outReq)
	if err != nil {
		return emptyResult(http.StatusBadGateway, "upstream"), nil
	}

	httpx.RemoveHopByHop(resp.Header)
	return &ResponseResult{
		Status:  resp.StatusCode,
		Headers: resp.Header,
		Body:    resp.Body,
		Source:  "upstream",
	}, nil
}

func readRequestBody(body io.ReadCloser, maxBytes int64) ([]byte, bool, error) {
	defer body.Close() //nolint:errcheck
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > maxBytes {
		return nil, true, nil
	}
	return data, false, nil
}

// dumpHeaders logs request/response headers at debug level, redacting sensitive values.
func dumpHeaders(logger *slog.Logger, direction string, h http.Header) {
	redact := map[string]struct{}{
		"Authorization": {},
		"Cookie":        {},
		"Set-Cookie":    {},
	}
	attrs := []any{
		slog.String(logging.KeyComponent, "dump"),
		slog.String("direction", direction),
	}
	for k, vals := range h {
		if _, sensitive := redact[k]; sensitive {
			attrs = append(attrs, slog.String(k, "[REDACTED]"))
			continue
		}
		for _, v := range vals {
			attrs = append(attrs, slog.String(k, v))
		}
	}
	logger.Debug("traffic_headers", attrs...)
}

// dumpBody wraps body so the first maxBytes bytes are logged then passed through.
func dumpBody(logger *slog.Logger, body io.ReadCloser, maxBytes int64) io.ReadCloser {
	if body == nil || maxBytes <= 0 {
		return body
	}
	snippet := &bytes.Buffer{}
	tee := io.TeeReader(body, &snippetLimitWriter{
		buf:       snippet,
		remaining: maxBytes,
	})
	return &dumpBodyReadCloser{
		body:    body,
		reader:  tee,
		logger:  logger,
		snippet: snippet,
	}
}

type snippetLimitWriter struct {
	buf       *bytes.Buffer
	remaining int64
}

func (w *snippetLimitWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return len(p), nil
	}
	n := len(p)
	if int64(n) > w.remaining {
		n = int(w.remaining)
	}
	if n > 0 {
		_, _ = w.buf.Write(p[:n])
		w.remaining -= int64(n)
	}
	return len(p), nil
}

type dumpBodyReadCloser struct {
	body    io.ReadCloser
	reader  io.Reader
	logger  *slog.Logger
	snippet *bytes.Buffer
	logged  bool
}

func (d *dumpBodyReadCloser) Read(p []byte) (int, error) {
	n, err := d.reader.Read(p)
	if err == io.EOF {
		d.logSnippet()
	}
	return n, err
}

func (d *dumpBodyReadCloser) Close() error {
	d.logSnippet()
	return d.body.Close()
}

func (d *dumpBodyReadCloser) logSnippet() {
	if d.logged {
		return
	}
	d.logged = true
	if d.snippet.Len() == 0 {
		return
	}
	d.logger.Debug("traffic_body",
		slog.String(logging.KeyComponent, "dump"),
		slog.String("snippet", d.snippet.String()),
		slog.Int("bytes", d.snippet.Len()),
	)
}

// WriteResponse writes result to w and logs the completed flow.
func WriteResponse(w http.ResponseWriter, result *ResponseResult, fc *FlowContext, start time.Time) {
	for k, vals := range result.Headers {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(result.Status)
	if result.Body != nil {
		io.Copy(w, result.Body) //nolint:errcheck
		result.Body.Close()     //nolint:errcheck
	}
	fc.Logger.Info("request",
		slog.String(logging.KeyComponent, "pipeline"),
		slog.String(logging.KeyClient, fc.ClientAddr),
		slog.String(logging.KeyMethod, fc.Method),
		slog.String(logging.KeyScheme, fc.Scheme),
		slog.String(logging.KeyHost, fc.Host),
		slog.String(logging.KeyPath, fc.Path),
		slog.Int(logging.KeyStatus, result.Status),
		slog.String(logging.KeySource, result.Source),
		slog.Int64(logging.KeyDurationMS, time.Since(start).Milliseconds()),
		slog.String(logging.KeyProto, fc.Proto),
	)
}
