package rules_test

import (
	"net/http"
	"testing"

	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/rules"
)

func compileOps(t *testing.T, raw *config.RawOps) *rules.Ops {
	t.Helper()
	ops, err := rules.CompileOps(raw)
	if err != nil {
		t.Fatalf("CompileOps: %v", err)
	}
	return ops
}

func TestOps_DeleteByName(t *testing.T) {
	h := http.Header{}
	h.Set("Cookie", "a=1")
	h.Add("Cookie", "b=2")
	h.Set("X-Keep", "yes")

	ops := compileOps(t, &config.RawOps{
		Delete: []config.RawDeleteOp{{Name: "Cookie"}},
	})
	ops.Apply(h)

	if len(h.Values("Cookie")) != 0 {
		t.Error("Cookie should be deleted")
	}
	if h.Get("X-Keep") != "yes" {
		t.Error("X-Keep should be preserved")
	}
}

func TestOps_DeleteByNameWithValueRegex(t *testing.T) {
	h := http.Header{}
	h.Add("Cookie", "session=abc")
	h.Add("Cookie", "prefs=xyz")

	ops := compileOps(t, &config.RawOps{
		Delete: []config.RawDeleteOp{{Name: "Cookie", ValueRegex: "^session="}},
	})
	ops.Apply(h)

	vals := h.Values("Cookie")
	if len(vals) != 1 || vals[0] != "prefs=xyz" {
		t.Errorf("expected [prefs=xyz], got %v", vals)
	}
}

func TestOps_WildcardDeleteByValueRegex(t *testing.T) {
	h := http.Header{}
	h.Set("X-Foo", "GDPR=")
	h.Set("X-Bar", "keep-me")
	h.Add("X-Baz", "GDPR=")
	h.Add("X-Baz", "also-keep")

	ops := compileOps(t, &config.RawOps{
		Delete: []config.RawDeleteOp{{AnyName: true, ValueRegex: "^GDPR=$"}},
	})
	ops.Apply(h)

	if len(h.Values("X-Foo")) != 0 {
		t.Errorf("X-Foo should be deleted, got %v", h.Values("X-Foo"))
	}
	if h.Get("X-Bar") != "keep-me" {
		t.Error("X-Bar should be preserved")
	}
	bazVals := h.Values("X-Baz")
	if len(bazVals) != 1 || bazVals[0] != "also-keep" {
		t.Errorf("X-Baz: expected [also-keep], got %v", bazVals)
	}
}

func TestOps_SetReplace(t *testing.T) {
	h := http.Header{}
	h.Set("X-Debug", "old")

	ops := compileOps(t, &config.RawOps{
		Set: []config.RawSetOp{{Name: "X-Debug", Mode: "replace", Value: "new"}},
	})
	ops.Apply(h)

	if h.Get("X-Debug") != "new" {
		t.Errorf("expected new, got %q", h.Get("X-Debug"))
	}
	if len(h.Values("X-Debug")) != 1 {
		t.Errorf("expected exactly 1 value, got %v", h.Values("X-Debug"))
	}
}

func TestOps_SetAppend(t *testing.T) {
	h := http.Header{}
	h.Set("X-Ids", "1")

	ops := compileOps(t, &config.RawOps{
		Set: []config.RawSetOp{{Name: "X-Ids", Mode: "append", Value: "2"}},
	})
	ops.Apply(h)

	vals := h.Values("X-Ids")
	if len(vals) != 2 || vals[0] != "1" || vals[1] != "2" {
		t.Errorf("expected [1, 2], got %v", vals)
	}
}

func TestOps_DeleteThenSet_Ordering(t *testing.T) {
	// Delete should run before set, even if set targets same header.
	h := http.Header{}
	h.Set("X-Debug", "old")

	ops := compileOps(t, &config.RawOps{
		Delete: []config.RawDeleteOp{{Name: "X-Debug"}},
		Set:    []config.RawSetOp{{Name: "X-Debug", Mode: "replace", Value: "new"}},
	})
	ops.Apply(h)

	if h.Get("X-Debug") != "new" {
		t.Errorf("expected new after delete+set, got %q", h.Get("X-Debug"))
	}
}

func TestCompileOps_InvalidSetMode(t *testing.T) {
	_, err := rules.CompileOps(&config.RawOps{
		Set: []config.RawSetOp{{Name: "X-Foo", Mode: "upsert", Value: "v"}},
	})
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestCompileOps_AnyNameWithoutValueRegex(t *testing.T) {
	_, err := rules.CompileOps(&config.RawOps{
		Delete: []config.RawDeleteOp{{AnyName: true}},
	})
	if err == nil {
		t.Fatal("expected error: any_name requires value")
	}
}

func TestCompileOps_Nil(t *testing.T) {
	ops, err := rules.CompileOps(nil)
	if err != nil {
		t.Fatalf("nil ops: %v", err)
	}
	h := http.Header{}
	h.Set("X-Foo", "bar")
	ops.Apply(h) // should be a no-op
	if h.Get("X-Foo") != "bar" {
		t.Error("nil ops should not modify headers")
	}
}
