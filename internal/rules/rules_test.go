package rules_test

import (
	"testing"

	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/rules"
)

func compileOrFatal(t *testing.T, rm config.RawMatch) *rules.Match {
	t.Helper()
	m, err := rules.CompileMatch(rm)
	if err != nil {
		t.Fatalf("CompileMatch: %v", err)
	}
	return m
}

func TestCompileMatch_ValidMethods(t *testing.T) {
	methods := []string{"GET", "HEAD", "POST", "PUT", "DELETE", "CONNECT", "OPTIONS", "TRACE", "PATCH"}
	for _, method := range methods {
		m := compileOrFatal(t, config.RawMatch{
			Methods: []string{method},
			Host:    ".*",
			Path:    ".*",
		})
		if _, ok := m.Methods[method]; !ok {
			t.Errorf("method %q not in compiled set", method)
		}
	}
}

func TestCompileMatch_InvalidMethod(t *testing.T) {
	_, err := rules.CompileMatch(config.RawMatch{
		Methods: []string{"BREW"},
		Host:    ".*",
		Path:    ".*",
	})
	if err == nil {
		t.Fatal("expected error for invalid method")
	}
}

func TestCompileMatch_MissingHost(t *testing.T) {
	_, err := rules.CompileMatch(config.RawMatch{Path: ".*"})
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestCompileMatch_MissingPath(t *testing.T) {
	_, err := rules.CompileMatch(config.RawMatch{Host: ".*"})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestCompileMatch_BadHostRegex(t *testing.T) {
	_, err := rules.CompileMatch(config.RawMatch{Host: "[invalid", Path: ".*"})
	if err == nil {
		t.Fatal("expected error for bad host regex")
	}
}

func TestCompileMatch_BadPathRegex(t *testing.T) {
	_, err := rules.CompileMatch(config.RawMatch{Host: ".*", Path: "[invalid"})
	if err == nil {
		t.Fatal("expected error for bad path regex")
	}
}

func TestMatches_HostWithoutPort(t *testing.T) {
	m := compileOrFatal(t, config.RawMatch{
		Host: `^example\.com$`,
		Path: `.*`,
	})

	if !m.Matches(rules.FlowMeta{Method: "GET", Scheme: "https", Host: "example.com", Path: "/"}) {
		t.Error("should match example.com")
	}
	// Host without port — port is in separate field
	if m.Matches(rules.FlowMeta{Method: "GET", Scheme: "https", Host: "example.com:443", Path: "/"}) {
		t.Error("should not match host with port embedded (caller must strip port)")
	}
}

func TestMatches_EmptyMethodsMatchesAll(t *testing.T) {
	m := compileOrFatal(t, config.RawMatch{Host: ".*", Path: ".*"})
	for _, method := range []string{"GET", "POST", "DELETE", "PATCH"} {
		if !m.Matches(rules.FlowMeta{Method: method, Host: "x", Path: "/"}) {
			t.Errorf("empty methods should match %q", method)
		}
	}
}

func TestMatches_MethodFilter(t *testing.T) {
	m := compileOrFatal(t, config.RawMatch{
		Methods: []string{"GET", "HEAD"},
		Host:    ".*",
		Path:    ".*",
	})
	if !m.Matches(rules.FlowMeta{Method: "GET", Host: "x", Path: "/"}) {
		t.Error("GET should match")
	}
	if m.Matches(rules.FlowMeta{Method: "POST", Host: "x", Path: "/"}) {
		t.Error("POST should not match")
	}
}

func TestMatches_PathRegex(t *testing.T) {
	m := compileOrFatal(t, config.RawMatch{
		Host: ".*",
		Path: `^/api/`,
	})
	if !m.Matches(rules.FlowMeta{Method: "GET", Host: "x", Path: "/api/v1"}) {
		t.Error("/api/v1 should match")
	}
	if m.Matches(rules.FlowMeta{Method: "GET", Host: "x", Path: "/static/foo"}) {
		t.Error("/static/foo should not match")
	}
}
