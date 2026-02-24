# Testing Patterns

**Analysis Date:** 2026-02-24

## Test Framework

**Runner:**
- Go stdlib `testing`
- Config: no custom test runner config file; execution via `go test`

**Assertion Style:**
- Native `testing.T` checks (`t.Fatalf`, `t.Errorf`, `t.Fatal`)

**Run Commands:**
```bash
go test ./... -timeout 120s        # Run all tests
make test                          # Wrapper for go test
make race                          # Race-enabled test run
```

## Test File Organization

**Location:**
- Tests are colocated with source files in each package directory (`internal/.../*_test.go`, `pkg/.../*_test.go`, `cmd/jeltz/*_test.go`)

**Naming:**
- File suffix `_test.go`
- Test function naming `Test<Subject>_<Behavior>`

**Structure:**
```text
internal/proxy/
  proxy.go
  proxy_test.go
  pipeline.go
  pipeline_test.go
  mitm.go
  mitm_h2_integration_test.go
```

## Test Structure

**Suite Organization:**
```go
func TestPipeline_MapLocal_ServesFile(t *testing.T) {
    dir := t.TempDir()
    rs := makeRuleSet(t, rawRules, dir)
    p := proxy.NewPipeline(rs, false)
    result, err := p.Run(flowContext)
    if err != nil { t.Fatalf(...) }
    if result.Status != http.StatusOK { t.Fatalf(...) }
}
```

**Patterns:**
- Setup with `t.TempDir()` for isolated filesystem state (`internal/config/config_test.go`, `internal/rules/maplocal_test.go`)
- Table-like iterative checks for method sets and variants (`internal/rules/rules_test.go`)
- Explicit helper constructors for repeated setup (`makeRuleSet`, `startTestProxy`, `buildCAPool` in proxy tests)

## Mocking

**Framework:**
- No external mocking framework

**Patterns:**
```go
upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // deterministic behavior
}))
defer upstream.Close()
```

**What to Mock:**
- Upstream HTTP/HTTPS services via `httptest.NewServer` / `httptest.NewTLSServer`
- TCP listeners and CONNECT tunnels via `net.Listen` / raw sockets

**What NOT to Mock:**
- Rule engine and pipeline core are exercised with real compiled rules
- TLS and certificate flows are validated with real generated CA and leaf certs

## Fixtures and Factories

**Test Data:**
```go
_ = os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0o644)
```

**Location:**
- Inline per-test fixtures in temp dirs; no central fixtures directory

## Coverage

**Requirements:**
- No explicit numeric threshold detected
- CI enforces broad regression coverage by running `go test ./...` on multiple OSes in `.github/workflows/test.yml`

**View Coverage:**
```bash
go test ./... -cover
```

## Test Types

**Unit Tests:**
- Config loading, rule compilation, header ops, CA helpers, XDG helpers (`internal/config`, `internal/rules`, `internal/ca`, `pkg/ca`, `pkg/xdg`)

**Integration Tests:**
- End-to-end proxy behaviors with CONNECT + MITM + ALPN and concurrent streams in `internal/proxy/mitm_h2_integration_test.go`
- Pipeline-to-upstream network behavior and timeout boundaries in `internal/proxy/pipeline_test.go`

**E2E Tests:**
- Not used as separate framework; integration tests provide in-process end-to-end coverage

## Common Patterns

**Async Testing:**
```go
var wg sync.WaitGroup
for i := range N {
    wg.Add(1)
    go func(i int) { defer wg.Done(); /* request */ }(i)
}
wg.Wait()
```

**Error Testing:**
```go
_, err := config.Load(path, tmp, tmp, config.CLIOverrides{})
if err == nil { t.Fatal("expected error") }
```

---

*Testing analysis: 2026-02-24*
