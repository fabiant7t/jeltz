package rules_test

import (
	"testing"

	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/rules"
)

func makeMapRemoteRule(t *testing.T, raw config.RawRule) *rules.MapRemoteRule {
	t.Helper()
	r, err := rules.CompileMapRemoteRule(raw)
	if err != nil {
		t.Fatalf("CompileMapRemoteRule: %v", err)
	}
	return r
}

func TestMapRemote_Resolve_MatchAndPrefixStripping(t *testing.T) {
	r := makeMapRemoteRule(t, config.RawRule{
		Type:  "map_remote",
		Match: config.RawMatch{Host: `^example\.com$`, Path: `^/api/`},
		URL:   "https://upstream.example.org/mock/",
	})

	target, err := r.Resolve(rules.FlowMeta{
		Method:   "GET",
		Host:     "example.com",
		Path:     "/api/v1/items",
		RawQuery: "x=1",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if target == nil {
		t.Fatal("expected non-nil target")
	}
	if target.Scheme != "https" {
		t.Fatalf("scheme: got %q", target.Scheme)
	}
	if target.Host != "upstream.example.org" {
		t.Fatalf("host: got %q", target.Host)
	}
	if target.Path != "/mock/v1/items" {
		t.Fatalf("path: got %q", target.Path)
	}
	if target.RawQuery != "x=1" {
		t.Fatalf("raw query: got %q", target.RawQuery)
	}
}

func TestMapRemote_Resolve_IncludesBaseQuery(t *testing.T) {
	r := makeMapRemoteRule(t, config.RawRule{
		Type:  "map_remote",
		Match: config.RawMatch{Host: `^example\.com$`, Path: `^/api/`},
		URL:   "https://upstream.example.org/mock?env=staging",
	})

	target, err := r.Resolve(rules.FlowMeta{
		Method:   "GET",
		Host:     "example.com",
		Path:     "/api/v1/items",
		RawQuery: "x=1",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if target == nil {
		t.Fatal("expected non-nil target")
	}
	if target.RawQuery != "env=staging&x=1" {
		t.Fatalf("raw query: got %q", target.RawQuery)
	}
}

func TestMapRemote_Resolve_NoMatch(t *testing.T) {
	r := makeMapRemoteRule(t, config.RawRule{
		Type:  "map_remote",
		Match: config.RawMatch{Host: `^example\.com$`, Path: `^/api/`},
		URL:   "https://upstream.example.org/mock/",
	})
	target, err := r.Resolve(rules.FlowMeta{Method: "GET", Host: "other.com", Path: "/api/x"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if target != nil {
		t.Fatal("expected nil target for non-match")
	}
}

func TestCompileMapRemoteRule_PathMustStartWithCaret(t *testing.T) {
	_, err := rules.CompileMapRemoteRule(config.RawRule{
		Type:  "map_remote",
		Match: config.RawMatch{Host: `.*`, Path: `/api/`},
		URL:   "https://upstream.example.org/mock/",
	})
	if err == nil {
		t.Fatal("expected error for match.path without caret")
	}
}

func TestCompileMapRemoteRule_RequiresURL(t *testing.T) {
	_, err := rules.CompileMapRemoteRule(config.RawRule{
		Type:  "map_remote",
		Match: config.RawMatch{Host: `.*`, Path: `^/`},
	})
	if err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestCompileMapRemoteRule_URLMustBeAbsolute(t *testing.T) {
	_, err := rules.CompileMapRemoteRule(config.RawRule{
		Type:  "map_remote",
		Match: config.RawMatch{Host: `.*`, Path: `^/`},
		URL:   "/relative/path",
	})
	if err == nil {
		t.Fatal("expected error for relative url")
	}
}
