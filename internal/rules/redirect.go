package rules

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"

	"github.com/fabiant7t/jeltz/internal/config"
)

const (
	RedirectSearchModeRegex   = "regex"
	RedirectSearchModeLiteral = "literal"
)

// RedirectRule is a compiled redirect rule.
type RedirectRule struct {
	Match         *Match
	SearchMode    string
	SearchRegex   *regexp.Regexp // only set for regex mode
	SearchLiteral string         // only set for literal mode
	Replace       string
	StatusCode    int
	Response      *Ops
}

// RedirectResult is returned when a RedirectRule is matched and rewrites a URL.
type RedirectResult struct {
	StatusCode int
	Location   string
	Response   *Ops
}

// CompileRedirectRule compiles a RawRule of type redirect.
func CompileRedirectRule(raw config.RawRule) (*RedirectRule, error) {
	m, err := CompileMatch(raw.Match)
	if err != nil {
		return nil, fmt.Errorf("match: %w", err)
	}

	if raw.Search == "" {
		return nil, fmt.Errorf("redirect rule requires search")
	}

	mode := raw.SearchMode
	if mode == "" {
		mode = RedirectSearchModeRegex
	}
	if mode != RedirectSearchModeRegex && mode != RedirectSearchModeLiteral {
		return nil, fmt.Errorf("search_mode must be regex or literal, got %q", mode)
	}

	statusCode := raw.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusFound
	}
	if statusCode < 300 || statusCode > 399 {
		return nil, fmt.Errorf("status_code must be a 3xx redirect status, got %d", statusCode)
	}

	if raw.ContentType != "" {
		return nil, fmt.Errorf("redirect rule does not support content_type")
	}

	respOps, err := CompileOps(raw.Response)
	if err != nil {
		return nil, fmt.Errorf("response ops: %w", err)
	}

	r := &RedirectRule{
		Match:      m,
		SearchMode: mode,
		Replace:    raw.Replace,
		StatusCode: statusCode,
		Response:   respOps,
	}
	if mode == RedirectSearchModeRegex {
		re, reErr := regexp.Compile(raw.Search)
		if reErr != nil {
			return nil, fmt.Errorf("search regex %q: %w", raw.Search, reErr)
		}
		r.SearchRegex = re
	} else {
		r.SearchLiteral = raw.Search
	}

	return r, nil
}

// Matches reports whether this rule applies to the current flow.
func (r *RedirectRule) Matches(fm FlowMeta) bool {
	return r.Match.Matches(fm)
}

// Resolve attempts to match fm and returns redirect metadata.
// Returns (nil, nil) if the rule does not match or does not rewrite the URL.
func (r *RedirectRule) Resolve(fm FlowMeta) (*RedirectResult, error) {
	if !r.Matches(fm) {
		return nil, nil
	}

	inputURL := fullFlowURL(fm)
	location := r.Apply(inputURL)
	if location == inputURL {
		return nil, nil
	}

	return &RedirectResult{
		StatusCode: r.StatusCode,
		Location:   location,
		Response:   r.Response,
	}, nil
}

// Apply returns the URL after replace-all search/replace.
func (r *RedirectRule) Apply(inputURL string) string {
	switch r.SearchMode {
	case RedirectSearchModeLiteral:
		return string(bytes.ReplaceAll([]byte(inputURL), []byte(r.SearchLiteral), []byte(r.Replace)))
	default:
		return string(r.SearchRegex.ReplaceAll([]byte(inputURL), []byte(r.Replace)))
	}
}

func fullFlowURL(fm FlowMeta) string {
	host := fm.Host
	if fm.Port != "" {
		host = fm.Host + ":" + fm.Port
	}

	path := fm.Path
	if path == "" {
		path = "/"
	}

	u := fm.Scheme + "://" + host + path
	if fm.RawQuery != "" {
		u += "?" + fm.RawQuery
	}
	return u
}
