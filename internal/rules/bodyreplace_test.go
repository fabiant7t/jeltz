package rules_test

import (
	"testing"

	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/rules"
)

func compileBodyReplaceRule(t *testing.T, raw config.RawRule) *rules.BodyReplaceRule {
	t.Helper()
	r, err := rules.CompileBodyReplaceRule(raw)
	if err != nil {
		t.Fatalf("CompileBodyReplaceRule: %v", err)
	}
	return r
}

func TestCompileBodyReplaceRule_DefaultsToRegex(t *testing.T) {
	r := compileBodyReplaceRule(t, config.RawRule{
		Type:    "body_replace",
		Match:   config.RawMatch{Host: `^example\.com$`, Path: `^/api/`},
		Search:  "foo(\\d+)",
		Replace: "bar-$1",
	})
	if r.SearchMode != rules.BodyReplaceSearchModeRegex {
		t.Fatalf("search mode: got %q", r.SearchMode)
	}
	if got := string(r.Apply([]byte("foo1 foo2"))); got != "bar-1 bar-2" {
		t.Fatalf("apply: got %q", got)
	}
}

func TestCompileBodyReplaceRule_LiteralMode(t *testing.T) {
	r := compileBodyReplaceRule(t, config.RawRule{
		Type:       "body_replace",
		Match:      config.RawMatch{Host: `^example\.com$`, Path: `^/api/`},
		Search:     "foo.*",
		SearchMode: "literal",
		Replace:    "x",
	})
	if got := string(r.Apply([]byte("foo.* foo.*"))); got != "x x" {
		t.Fatalf("apply: got %q", got)
	}
}

func TestCompileBodyReplaceRule_InvalidSearchMode(t *testing.T) {
	_, err := rules.CompileBodyReplaceRule(config.RawRule{
		Type:       "body_replace",
		Match:      config.RawMatch{Host: `.*`, Path: `.*`},
		Search:     "foo",
		SearchMode: "glob",
		Replace:    "bar",
	})
	if err == nil {
		t.Fatal("expected error for invalid search_mode")
	}
}

func TestCompileBodyReplaceRule_InvalidSearchRegex(t *testing.T) {
	_, err := rules.CompileBodyReplaceRule(config.RawRule{
		Type:    "body_replace",
		Match:   config.RawMatch{Host: `.*`, Path: `.*`},
		Search:  "[bad",
		Replace: "x",
	})
	if err == nil {
		t.Fatal("expected error for invalid search regex")
	}
}

func TestCompileBodyReplaceRule_InvalidContentTypeRegex(t *testing.T) {
	_, err := rules.CompileBodyReplaceRule(config.RawRule{
		Type:        "body_replace",
		Match:       config.RawMatch{Host: `.*`, Path: `.*`},
		Search:      "foo",
		Replace:     "bar",
		ContentType: "[bad",
	})
	if err == nil {
		t.Fatal("expected error for invalid content_type regex")
	}
}

func TestBodyReplaceRule_MatchesContentTypeFilter(t *testing.T) {
	r := compileBodyReplaceRule(t, config.RawRule{
		Type:        "body_replace",
		Match:       config.RawMatch{Host: `^example\.com$`, Path: `^/`},
		Search:      "foo",
		Replace:     "bar",
		ContentType: `^text/`,
	})
	fm := rules.FlowMeta{Method: "GET", Host: "example.com", Path: "/x"}
	if !r.Matches(fm, "text/plain; charset=utf-8") {
		t.Fatal("expected text/plain to match content_type filter")
	}
	if r.Matches(fm, "application/json") {
		t.Fatal("expected application/json to not match content_type filter")
	}
}

func TestBodyReplaceRule_ReplaceCanBeEmpty(t *testing.T) {
	r := compileBodyReplaceRule(t, config.RawRule{
		Type:    "body_replace",
		Match:   config.RawMatch{Host: `.*`, Path: `.*`},
		Search:  "foo",
		Replace: "",
	})
	if got := string(r.Apply([]byte("foo123foo"))); got != "123" {
		t.Fatalf("apply: got %q", got)
	}
}
