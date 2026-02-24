// Package rules implements the jeltz rule engine.
package rules

import (
	"fmt"
	"regexp"

	"github.com/fabiant7t/jeltz/internal/config"
)

// validMethods is the set of HTTP methods jeltz accepts in rule configs.
var validMethods = map[string]struct{}{
	"GET":     {},
	"HEAD":    {},
	"POST":    {},
	"PUT":     {},
	"DELETE":  {},
	"CONNECT": {},
	"OPTIONS": {},
	"TRACE":   {},
	"PATCH":   {},
}

// FlowMeta holds the metadata for a single proxied request.
type FlowMeta struct {
	Method          string
	Scheme          string // "http" or "https"
	Host            string // hostname only, without port
	Port            string
	Path            string
	RawQuery        string
	FullPathWithQuery string
}

// Match is a compiled rule matcher.
type Match struct {
	Methods map[string]struct{} // empty = any method
	Host    *regexp.Regexp
	Path    *regexp.Regexp
}

// Matches reports whether fm satisfies all conditions of m.
func (m *Match) Matches(fm FlowMeta) bool {
	if len(m.Methods) > 0 {
		if _, ok := m.Methods[fm.Method]; !ok {
			return false
		}
	}
	if !m.Host.MatchString(fm.Host) {
		return false
	}
	if !m.Path.MatchString(fm.Path) {
		return false
	}
	return true
}

// CompileMatch compiles a RawMatch into a Match, validating methods and regexes.
func CompileMatch(rm config.RawMatch) (*Match, error) {
	methods := make(map[string]struct{}, len(rm.Methods))
	for _, method := range rm.Methods {
		if _, ok := validMethods[method]; !ok {
			return nil, fmt.Errorf("unsupported HTTP method %q", method)
		}
		methods[method] = struct{}{}
	}

	if rm.Host == "" {
		return nil, fmt.Errorf("match.host is required")
	}
	hostRe, err := regexp.Compile(rm.Host)
	if err != nil {
		return nil, fmt.Errorf("match.host regex %q: %w", rm.Host, err)
	}

	if rm.Path == "" {
		return nil, fmt.Errorf("match.path is required")
	}
	pathRe, err := regexp.Compile(rm.Path)
	if err != nil {
		return nil, fmt.Errorf("match.path regex %q: %w", rm.Path, err)
	}

	return &Match{
		Methods: methods,
		Host:    hostRe,
		Path:    pathRe,
	}, nil
}
