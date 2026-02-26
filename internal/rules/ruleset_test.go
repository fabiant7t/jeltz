package rules_test

import (
	"testing"

	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/rules"
)

func boolPtr(v bool) *bool { return &v }

func TestCompile_DefaultEnabledTrue(t *testing.T) {
	rs, err := rules.Compile([]config.RawRule{
		{
			Type:  "header",
			Match: config.RawMatch{Host: `.*`, Path: `.*`},
		},
	}, t.TempDir())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got, want := len(rs.Headers), 1; got != want {
		t.Fatalf("headers len: got %d, want %d", got, want)
	}
}

func TestCompile_DisabledRuleIsSkipped(t *testing.T) {
	rs, err := rules.Compile([]config.RawRule{
		{
			Type:    "header",
			Enabled: boolPtr(false),
			Match:   config.RawMatch{Host: `.*`, Path: `.*`},
		},
		{
			Type:  "header",
			Match: config.RawMatch{Host: `.*`, Path: `.*`},
		},
	}, t.TempDir())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got, want := len(rs.Headers), 1; got != want {
		t.Fatalf("headers len: got %d, want %d", got, want)
	}
}

func TestCompile_DisabledInvalidRuleDoesNotFailCompile(t *testing.T) {
	rs, err := rules.Compile([]config.RawRule{
		{
			Type:    "map_local",
			Enabled: boolPtr(false),
			Match:   config.RawMatch{Host: `.*`, Path: `^/`},
			Path:    "/this/path/does/not/exist",
		},
		{
			Type:  "header",
			Match: config.RawMatch{Host: `.*`, Path: `.*`},
		},
	}, t.TempDir())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := len(rs.MapLocal); got != 0 {
		t.Fatalf("map_local len: got %d, want 0", got)
	}
	if got := len(rs.Headers); got != 1 {
		t.Fatalf("headers len: got %d, want 1", got)
	}
}

func TestCompile_DisabledUnknownTypeIsIgnored(t *testing.T) {
	rs, err := rules.Compile([]config.RawRule{
		{
			Type:    "not_a_rule",
			Enabled: boolPtr(false),
			Match:   config.RawMatch{Host: `.*`, Path: `.*`},
		},
		{
			Type:  "header",
			Match: config.RawMatch{Host: `.*`, Path: `.*`},
		},
	}, t.TempDir())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got, want := len(rs.Headers), 1; got != want {
		t.Fatalf("headers len: got %d, want %d", got, want)
	}
}
