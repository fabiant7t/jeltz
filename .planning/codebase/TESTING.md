# Testing

**Analysis Date:** 2026-02-24

## Test Framework(s)

**Runner:** Go's built-in `testing` package — no third-party test framework or assertion library.

**Assertions:** Standard `t.Errorf`, `t.Fatalf`, `t.Error`, `t.Fatal` — no testify or gomock.

**Race detector:** Supported via `make race` (`go test -race ./...`).

**Timeout:** All test runs use `-timeout 120s` (set in Makefile).

## Test Types Present

**Unit tests:** Cover pure logic in isolation — rule compilation, header operations, config loading, CA/PKI operations, XDG path resolution. Examples: `internal/rules/rules_test.go`, `internal/rules/headers_test.go`, `internal/config/config_test.go`, `pkg/ca/ca_test.go`, `pkg/xdg/xdg_test.go`.

**Integration tests (in-process):** `internal/proxy/pipeline_test.go` tests the full pipeline using `httptest.NewServer` as an upstream and `proxy.FlowContext` directly — no real network sockets needed for most cases.

**End-to-end integration tests:** `internal/proxy/mitm_h2_integration_test.go` starts a real in-process jeltz proxy on an ephemeral port, establishes TLS CONNECT tunnels, negotiates ALPN, and sends real HTTP/1.1 and HTTP/2 requests through the proxy. Tests both map-local serving and upstream passthrough with TLS.

**External tool tests (conditional):** `pkg/p12/p12_test.go` includes `TestEncode_Openssl` which invokes the `openssl` binary via `exec.Command` to verify PKCS#12 bundles. The test is skipped when `openssl` is not in PATH using `t.Skip`.

## Test Organization

**Location:** Test files are co-located with source files in the same directory.

**Package naming:** All test files use the external test package pattern (`package <pkg>_test`), e.g.:
- `internal/rules/rules_test.go` → `package rules_test`
- `internal/config/config_test.go` → `package config_test`
- `internal/ca/ca_test.go` → `package ca_test`
- `pkg/ca/ca_test.go` → `package ca_test`

This means tests always use the exported API, not unexported internals.

**Test file naming:**
- Unit/integration tests: `<source_file>_test.go` (e.g., `rules_test.go`, `headers_test.go`, `maplocal_test.go`)
- Dedicated integration test file: `mitm_h2_integration_test.go` (also in `package proxy_test`)

**Test function naming:** `Test<Type>_<Scenario>` convention:
- `TestCompileMatch_ValidMethods`
- `TestOps_DeleteByNameWithValueRegex`
- `TestLoad_CLIOverrides`
- `TestMITM_H2_ConcurrentStreams`
- `TestLeafCert_DiskCache`

**Test helper functions:**
All helpers call `t.Helper()` as their first line. Helpers are defined as unexported functions within the test file:
- `compileOrFatal(t, rm)` — compiles a `RawMatch` or fails the test
- `compileOps(t, raw)` — compiles `RawOps` or fails the test
- `makeMapLocalRule(t, basePath, matchPath, fsPath)` — compiles a map-local rule or fails
- `writeConfig(t, dir, content)` — writes a YAML config file to a temp dir
- `testLogger()` — returns a `*slog.Logger` writing to `io.Discard`
- `startTestProxy(t, rawRules, basePath)` — starts a full in-process proxy, returns addr + CA
- `buildCAPool(t, caInst)` — builds an `x509.CertPool` from the CA cert
- `connectTunnel(t, proxyAddr, target)` — sends a CONNECT request and returns the raw conn
- `testKey(t)` / `testCert(t, key)` — generate RSA key and self-signed cert for p12 tests

## Coverage Approach

**Coverage tooling:** No enforced coverage threshold. Coverage can be viewed with:
```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```
(No coverage target in Makefile; not enforced in CI.)

**What is tested:**
- Rule compilation and matching logic (methods, host regex, path regex): `internal/rules/rules_test.go`
- Header operations (delete by name, delete by value regex, wildcard delete, set/replace/append, ordering): `internal/rules/headers_test.go`
- Map-local resolution (prefix stripping, index file, directory vs file, path traversal protection, content type detection): `internal/rules/maplocal_test.go`
- Config loading (defaults, YAML parsing, strict field validation, version check, CLI overrides, path resolution): `internal/config/config_test.go`
- CA lifecycle (create, load, idempotency, leaf cert issuance, memory cache, disk cache, chain verification): `internal/ca/ca_test.go`
- Low-level CA/PKI functions (key size, CommonName, IsCA, key usage, validity period, self-signed verification, DNS/IP SANs, chain length, leaf verification): `pkg/ca/ca_test.go`
- PKCS#12 encoding (basic encode, empty password, openssl interop): `pkg/p12/p12_test.go`
- XDG path resolution (XDG env var, HOME fallback for config and data dirs): `pkg/xdg/xdg_test.go`
- Pipeline integration (map-local serving, response header rules, request header transforms, upstream passthrough, 404 on missing file, combined global + map-local response ops, `WriteResponse`): `internal/proxy/pipeline_test.go`
- Full proxy stack (MITM with ALPN h2 negotiation, 20 concurrent H2 streams, HTTP/1.1 fallback, upstream TLS passthrough with header injection): `internal/proxy/mitm_h2_integration_test.go`

**What is not explicitly tested:**
- `cmd/jeltz/main.go` entry point (no test file)
- `cmd/jeltz/banner.go` (no test file)
- `internal/proxy/proxy.go` CONNECT handler (covered indirectly by integration tests but no isolated unit tests)
- `internal/httpx/hopbyhop.go` (no test file)
- `internal/logging/logging.go` (no test file)

## How to Run Tests

```bash
# Run all tests
make test
# equivalent: go test ./... -timeout 120s

# Run with race detector
make race
# equivalent: go test -race ./... -timeout 120s

# Run a specific package
go test github.com/fabiant7t/jeltz/internal/rules -timeout 120s

# Run a single test by name
go test ./internal/proxy/... -run TestMITM_H2_ConcurrentStreams -timeout 120s

# Run with verbose output
go test -v ./... -timeout 120s

# Run linter
make lint
# equivalent: go vet ./...
```
