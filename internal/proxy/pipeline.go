package proxy

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
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
	Header     http.Header   // mutable request headers
	Body       io.ReadCloser // may be nil
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
	ruleset          *rules.RuleSet
	insecureUpstream bool
}

// NewPipeline creates a Pipeline.
func NewPipeline(rs *rules.RuleSet, insecureUpstream bool) *Pipeline {
	return &Pipeline{ruleset: rs, insecureUpstream: insecureUpstream}
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

	// Step 3: choose body source — map-local first-match, else upstream.
	var result *ResponseResult
	var mapLocalOps *rules.Ops

	if p.ruleset != nil {
		for _, ml := range p.ruleset.MapLocal {
			mlResult, err := ml.Resolve(fm)
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
				mapLocalOps = mlResult.Response
				result = r
				break
			}
		}
	}

	if result == nil {
		var err error
		result, err = p.roundtrip(fc)
		if err != nil {
			return nil, err
		}
	}

	// Step 4: apply matching response header rules.
	if p.ruleset != nil {
		for _, hr := range p.ruleset.Headers {
			if hr.Match.Matches(fm) {
				hr.Response.Apply(result.Headers)
			}
		}
	}

	// Step 5: apply map-local response ops after global response rules.
	if mapLocalOps != nil {
		mapLocalOps.Apply(result.Headers)
	}

	return result, nil
}

func emptyResult(status int, source string) *ResponseResult {
	return &ResponseResult{
		Status:  status,
		Headers: make(http.Header),
		Body:    io.NopCloser(bytes.NewReader(nil)),
		Source:  source,
	}
}

// serveLocal builds a ResponseResult from a MapLocalResult.
func serveLocal(mlr *rules.MapLocalResult) (*ResponseResult, error) {
	data, err := os.ReadFile(mlr.FSTarget)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyResult(http.StatusNotFound, "local"), nil
		}
		return nil, fmt.Errorf("reading local file %q: %w", mlr.FSTarget, err)
	}

	ct := rules.DetectContentType(mlr.FSTarget, mlr.ContentType, func(p string) ([]byte, error) {
		return data[:min(512, len(data))], nil
	})

	h := make(http.Header)
	h.Set("Content-Type", ct)

	return &ResponseResult{
		Status:  mlr.StatusCode,
		Headers: h,
		Body:    io.NopCloser(bytes.NewReader(data)),
		Source:  "local",
	}, nil
}

// roundtrip performs an upstream HTTP request using fc's context.
func (p *Pipeline) roundtrip(fc *FlowContext) (*ResponseResult, error) {
	host := fc.Host
	if fc.Port != "" {
		host = fc.Host + ":" + fc.Port
	}
	targetURL := fc.Scheme + "://" + host + fc.Path
	if fc.RawQuery != "" {
		targetURL += "?" + fc.RawQuery
	}

	var body io.Reader
	if fc.Body != nil {
		body = fc.Body
	}
	outReq, err := http.NewRequest(fc.Method, targetURL, body)
	if err != nil {
		return nil, fmt.Errorf("building upstream request: %w", err)
	}
	for k, vals := range fc.Header {
		for _, v := range vals {
			outReq.Header.Add(k, v)
		}
	}
	httpx.RemoveHopByHop(outReq.Header)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: p.insecureUpstream, //nolint:gosec
		},
		Proxy: nil,
	}
	resp, err := tr.RoundTrip(outReq)
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
		result.Body.Close()    //nolint:errcheck
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
