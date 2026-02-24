package rules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabiant7t/jeltz/internal/config"
	"github.com/fabiant7t/jeltz/internal/rules"
)

func makeMapLocalRule(t *testing.T, basePath, matchPath, fsPath string) *rules.MapLocalRule {
	t.Helper()
	raw := config.RawRule{
		Type: "map_local",
		Match: config.RawMatch{
			Host: `^example\.com$`,
			Path: matchPath,
		},
		Path: fsPath,
	}
	r, err := rules.CompileMapLocalRule(raw, basePath)
	if err != nil {
		t.Fatalf("CompileMapLocalRule: %v", err)
	}
	return r
}

func TestMapLocal_PrefixStripping_Dir(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "static")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := makeMapLocalRule(t, dir, `^/static/`, subDir)
	result, err := r.Resolve(rules.FlowMeta{Method: "GET", Host: "example.com", Path: "/static/file.txt"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result == nil {
		t.Fatal("expected match, got nil")
	}
	if result.FSTarget != filepath.Join(subDir, "file.txt") {
		t.Errorf("FSTarget: got %q", result.FSTarget)
	}
}

func TestMapLocal_PrefixStripping_IndexFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := makeMapLocalRule(t, dir, `^/app/`, dir)
	// /app/ → stripped = "/" → "/" ends with "/" → append index.html
	result, err := r.Resolve(rules.FlowMeta{Method: "GET", Host: "example.com", Path: "/app/"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result == nil {
		t.Fatal("expected match")
	}
	if result.FSTarget != filepath.Join(dir, "index.html") {
		t.Errorf("FSTarget: got %q", result.FSTarget)
	}
}

func TestMapLocal_NoMatch_WrongHost(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := makeMapLocalRule(t, dir, `^/static/`, dir)
	result, err := r.Resolve(rules.FlowMeta{Method: "GET", Host: "other.com", Path: "/static/x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil for non-matching host")
	}
}

func TestMapLocal_NoMatch_PathNotFromStart(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Use a path regex that doesn't start with ^ to verify runtime enforcement.
	// Actually CompileMapLocalRule requires ^ so we test the runtime check
	// via a regex that matches mid-string when ^ is removed by using a workaround.
	// Instead, test that an empty stripped part (after full path consumed) works.
	r := makeMapLocalRule(t, dir, `^/static`, dir)
	// Path "/other/static" — regex matches at index 0? No, ^ anchors to start so it won't.
	result, err := r.Resolve(rules.FlowMeta{Method: "GET", Host: "example.com", Path: "/other/static"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil for non-matching path")
	}
}

func TestMapLocal_TraversalProtection_URLDotDot(t *testing.T) {
	dir := t.TempDir()
	// Write a file inside the rule dir.
	_ = os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("ok"), 0o644)
	// Write a "secret" in a sibling dir that should never be reachable.
	sibling := t.TempDir()
	_ = os.WriteFile(filepath.Join(sibling, "secret.txt"), []byte("secret"), 0o644)

	r := makeMapLocalRule(t, dir, `^/static/`, dir)
	// Attempt URL-level traversal: /static/../../secret.txt
	// path.Clean("/" + "../../secret.txt") → "/secret.txt" → stays within dir.
	// No traversal error; the resolved path is dir/secret.txt (which may not exist).
	result, err := r.Resolve(rules.FlowMeta{Method: "GET", Host: "example.com", Path: "/static/../../secret.txt"})
	if err != nil {
		// Traversal detected at filesystem level — also acceptable.
		if !rules.IsTraversal(err) {
			t.Errorf("unexpected error: %v", err)
		}
		return
	}
	if result == nil {
		t.Fatal("expected a result (match occurred)")
	}
	// The resolved target must be within dir, not in sibling.
	rel, err2 := filepath.Rel(dir, result.FSTarget)
	if err2 != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("resolved path %q escapes rule dir %q", result.FSTarget, dir)
	}
}

func TestMapLocal_PathMustExistAtCompile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")
	raw := config.RawRule{
		Type:  "map_local",
		Match: config.RawMatch{Host: ".*", Path: "^/"},
		Path:  missing,
	}
	_, err := rules.CompileMapLocalRule(raw, dir)
	if err == nil {
		t.Fatal("expected error for missing map_local path")
	}
}

func TestMapLocal_PathMustStartWithCaret(t *testing.T) {
	dir := t.TempDir()
	raw := config.RawRule{
		Type:  "map_local",
		Match: config.RawMatch{Host: ".*", Path: "/static/"},
		Path:  dir,
	}
	_, err := rules.CompileMapLocalRule(raw, dir)
	if err == nil {
		t.Fatal("expected error: path regex must start with ^")
	}
}

func TestMapLocal_FileRuleServesDirectly(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "data.json")
	if err := os.WriteFile(f, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	raw := config.RawRule{
		Type:  "map_local",
		Match: config.RawMatch{Host: `^example\.com$`, Path: `^/api/data`},
		Path:  f,
	}
	r, err := rules.CompileMapLocalRule(raw, dir)
	if err != nil {
		t.Fatalf("CompileMapLocalRule: %v", err)
	}
	result, err := r.Resolve(rules.FlowMeta{Method: "GET", Host: "example.com", Path: "/api/data"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result == nil {
		t.Fatal("expected match")
	}
	if result.FSTarget != f {
		t.Errorf("FSTarget: got %q, want %q", result.FSTarget, f)
	}
}

func TestDetectContentType_Override(t *testing.T) {
	ct := rules.DetectContentType("file.bin", "application/json", nil)
	if ct != "application/json" {
		t.Errorf("expected override, got %q", ct)
	}
}

func TestDetectContentType_Extension(t *testing.T) {
	ct := rules.DetectContentType("file.html", "", nil)
	if ct == "" || ct == "application/octet-stream" {
		t.Errorf("expected mime from extension, got %q", ct)
	}
}

func TestDetectContentType_Sniff(t *testing.T) {
	ct := rules.DetectContentType("file.bin", "", func(p string) ([]byte, error) {
		return []byte(`{"key":"value"}`), nil
	})
	// DetectContentType on JSON-like content returns text/plain
	if ct == "" {
		t.Error("expected non-empty content type from sniff")
	}
}
