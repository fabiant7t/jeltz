package rules_test

import (
	"testing"

	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/rules"
)

func compileRedirectRule(t *testing.T, raw config.RawRule) *rules.RedirectRule {
	t.Helper()
	r, err := rules.CompileRedirectRule(raw)
	if err != nil {
		t.Fatalf("CompileRedirectRule: %v", err)
	}
	return r
}

func TestCompileRedirectRule_Defaults(t *testing.T) {
	r := compileRedirectRule(t, config.RawRule{
		Type:    "redirect",
		Match:   config.RawMatch{Host: `^example\.com$`, Path: `^/old/`},
		Search:  `^https://example\.com/old/(.*)$`,
		Replace: `https://example.com/new/$1`,
	})
	if r.SearchMode != rules.RedirectSearchModeRegex {
		t.Fatalf("search mode: got %q", r.SearchMode)
	}
	if r.StatusCode != 302 {
		t.Fatalf("status code: got %d", r.StatusCode)
	}
}

func TestCompileRedirectRule_LiteralMode(t *testing.T) {
	r := compileRedirectRule(t, config.RawRule{
		Type:       "redirect",
		Match:      config.RawMatch{Host: `^example\.com$`, Path: `^/`},
		Search:     "example.com",
		SearchMode: "literal",
		Replace:    "mirror.example.com",
	})
	got := r.Apply("https://example.com/foo")
	if got != "https://mirror.example.com/foo" {
		t.Fatalf("apply: got %q", got)
	}
}

func TestCompileRedirectRule_InvalidSearchMode(t *testing.T) {
	_, err := rules.CompileRedirectRule(config.RawRule{
		Type:       "redirect",
		Match:      config.RawMatch{Host: `.*`, Path: `.*`},
		Search:     "x",
		SearchMode: "glob",
		Replace:    "y",
	})
	if err == nil {
		t.Fatal("expected error for invalid search_mode")
	}
}

func TestCompileRedirectRule_InvalidSearchRegex(t *testing.T) {
	_, err := rules.CompileRedirectRule(config.RawRule{
		Type:    "redirect",
		Match:   config.RawMatch{Host: `.*`, Path: `.*`},
		Search:  "[bad",
		Replace: "x",
	})
	if err == nil {
		t.Fatal("expected error for invalid search regex")
	}
}

func TestCompileRedirectRule_InvalidContentTypeRegex(t *testing.T) {
	_, err := rules.CompileRedirectRule(config.RawRule{
		Type:        "redirect",
		Match:       config.RawMatch{Host: `.*`, Path: `.*`},
		Search:      "foo",
		Replace:     "bar",
		ContentType: "[bad",
	})
	if err == nil {
		t.Fatal("expected error for invalid content_type regex")
	}
}

func TestCompileRedirectRule_StatusCodeMustBe3xx(t *testing.T) {
	_, err := rules.CompileRedirectRule(config.RawRule{
		Type:       "redirect",
		Match:      config.RawMatch{Host: `.*`, Path: `.*`},
		Search:     "foo",
		Replace:    "bar",
		StatusCode: 200,
	})
	if err == nil {
		t.Fatal("expected error for non-3xx status code")
	}
}

func TestRedirectRule_Resolve_RewritesFullURL(t *testing.T) {
	r := compileRedirectRule(t, config.RawRule{
		Type:    "redirect",
		Match:   config.RawMatch{Host: `^example\.com$`, Path: `^/v1/`},
		Search:  `^https://example\.com/v1/(.*)$`,
		Replace: `https://api.example.com/v2/$1`,
	})

	result, err := r.Resolve(rules.FlowMeta{
		Method:   "GET",
		Scheme:   "https",
		Host:     "example.com",
		Path:     "/v1/items",
		RawQuery: "q=1",
	}, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if got, want := result.Location, "https://api.example.com/v2/items?q=1"; got != want {
		t.Fatalf("location: got %q, want %q", got, want)
	}
	if got, want := result.StatusCode, 302; got != want {
		t.Fatalf("status: got %d, want %d", got, want)
	}
}

func TestRedirectRule_Resolve_NoRewriteReturnsNil(t *testing.T) {
	r := compileRedirectRule(t, config.RawRule{
		Type:    "redirect",
		Match:   config.RawMatch{Host: `^example\.com$`, Path: `^/`},
		Search:  "does-not-match",
		Replace: "x",
	})

	result, err := r.Resolve(rules.FlowMeta{
		Method: "GET", Scheme: "https", Host: "example.com", Path: "/",
	}, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result")
	}
}

func TestRedirectRule_Resolve_ContentTypeFilter(t *testing.T) {
	r := compileRedirectRule(t, config.RawRule{
		Type:        "redirect",
		Match:       config.RawMatch{Host: `^example\.com$`, Path: `^/`},
		Search:      "example.com",
		Replace:     "alt.example.com",
		ContentType: `^application/json`,
	})

	fm := rules.FlowMeta{Method: "POST", Scheme: "https", Host: "example.com", Path: "/submit"}
	miss, err := r.Resolve(fm, "text/plain")
	if err != nil {
		t.Fatalf("Resolve text/plain: %v", err)
	}
	if miss != nil {
		t.Fatal("expected nil result for non-matching content type")
	}

	hit, err := r.Resolve(fm, "application/json; charset=utf-8")
	if err != nil {
		t.Fatalf("Resolve application/json: %v", err)
	}
	if hit == nil {
		t.Fatal("expected redirect result")
	}
}
