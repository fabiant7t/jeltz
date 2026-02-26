package rules_test

import (
	"encoding/base64"
	"testing"

	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/rules"
)

func makeMapRule(t *testing.T, raw config.RawRule) *rules.MapRule {
	t.Helper()
	r, err := rules.CompileMapRule(raw)
	if err != nil {
		t.Fatalf("CompileMapRule: %v", err)
	}
	return r
}

func TestCompileMapRule_TextBody(t *testing.T) {
	r := makeMapRule(t, config.RawRule{
		Type:        "map",
		Match:       config.RawMatch{Host: `^example\.com$`, Path: `^/api/`},
		Body:        "hello",
		StatusCode:  201,
		ContentType: "text/plain",
	})
	res, err := r.Resolve(rules.FlowMeta{Method: "GET", Host: "example.com", Path: "/api/x"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if got, want := string(res.Body), "hello"; got != want {
		t.Fatalf("body: got %q, want %q", got, want)
	}
	if got, want := res.StatusCode, 201; got != want {
		t.Fatalf("status: got %d, want %d", got, want)
	}
}

func TestCompileMapRule_Base64Body(t *testing.T) {
	bin := []byte{0x00, 0x01, 0x02, 0xff}
	r := makeMapRule(t, config.RawRule{
		Type:       "map",
		Match:      config.RawMatch{Host: `.*`, Path: `.*`},
		BodyBase64: base64.StdEncoding.EncodeToString(bin),
	})
	res, err := r.Resolve(rules.FlowMeta{Method: "GET", Host: "example.com", Path: "/"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if got, want := string(res.Body), string(bin); got != want {
		t.Fatalf("body mismatch: got %v, want %v", res.Body, bin)
	}
}

func TestCompileMapRule_RequiresExactlyOneBodyField(t *testing.T) {
	_, err := rules.CompileMapRule(config.RawRule{
		Type:  "map",
		Match: config.RawMatch{Host: `.*`, Path: `.*`},
	})
	if err == nil {
		t.Fatal("expected error with no body/body_base64")
	}

	_, err = rules.CompileMapRule(config.RawRule{
		Type:       "map",
		Match:      config.RawMatch{Host: `.*`, Path: `.*`},
		Body:       "x",
		BodyBase64: "eA==",
	})
	if err == nil {
		t.Fatal("expected error with both body and body_base64")
	}
}

func TestCompileMapRule_InvalidBase64(t *testing.T) {
	_, err := rules.CompileMapRule(config.RawRule{
		Type:       "map",
		Match:      config.RawMatch{Host: `.*`, Path: `.*`},
		BodyBase64: "!!!",
	})
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestMapRule_NoMatch(t *testing.T) {
	r := makeMapRule(t, config.RawRule{
		Type:  "map",
		Match: config.RawMatch{Host: `^example\.com$`, Path: `^/api/`},
		Body:  "hello",
	})
	res, err := r.Resolve(rules.FlowMeta{Method: "GET", Host: "other.com", Path: "/api/x"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res != nil {
		t.Fatal("expected nil result for non-match")
	}
}
