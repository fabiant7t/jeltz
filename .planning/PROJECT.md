# jeltz

## What This Is

A single-binary TLS-intercepting HTTP/HTTPS forward proxy (MITM proxy) for local development. It issues per-host leaf certificates on demand, applies configurable request/response header rules, and can serve local files in place of upstream responses. Aimed at developers who need to inspect or modify browser traffic without installing a heavyweight tool.

## Core Value

Intercept and modify HTTPS traffic transparently — any rule change takes effect without touching the browser or OS trust store again.

## Requirements

### Validated

- ✓ MITM interception of HTTPS via dynamic per-host leaf certificate issuance — existing
- ✓ Plain HTTP forward proxy support — existing
- ✓ YAML config file with `JELTZ_*` env var and CLI flag override layers — existing
- ✓ Request and response header manipulation rules (add/delete) — existing
- ✓ `map_local` rule: serve local files instead of upstream responses — existing
- ✓ Traffic dump (`-dump-traffic` flag) for request/response inspection — existing
- ✓ HTTP/2 support on the MITM tunnel leg via `x/net/http2` — existing
- ✓ CA management subcommands (`ca-path`, `ca-p12-path`, `ca-install-hint`) — existing
- ✓ PKCS#12 bundle export for browser/OS CA import — existing
- ✓ Graceful shutdown on SIGINT/SIGTERM — existing
- ✓ XDG Base Directory support for config and data paths — existing

### Active

- [x] `handleForward` (plain HTTP forward handler) has isolated unit tests covering the no-pipeline fallback and the pipeline-integrated path
- [x] `rawTunnel` (non-MITM CONNECT TCP fallback) has unit tests covering the bidirectional copy logic
- [x] CLI boolean override semantics fixed: `-insecure-upstream` and `-dump-traffic` only override YAML when explicitly set

### Out of Scope

- Upstream transport timeouts — not requested
- `map_local` streaming via `http.ServeContent` — not requested
- `io.TeeReader` fix for `-dump-traffic` truncation — not requested
- Per-host cert cache eviction — not requested
- Windows build target — not requested

## Current Milestone: v1.2 — CLI Override Semantics

**Goal:** Ensure boolean CLI flags do not silently override YAML defaults unless explicitly passed.

**Target features:**
- `cmd/jeltz/main.go` only sets `CLIOverrides` bool pointers when corresponding flags were visited
- Unit tests for explicit-vs-implicit flag override behavior in `cmd/jeltz/main_test.go`

## Completed Milestone: v1.1 — Proxy Handler Tests

**Goal:** Close the test coverage gap on the two untested proxy handlers in `internal/proxy/proxy.go`.

**Target features:**
- Unit tests for `handleForward` (no-pipeline fallback + pipeline path)
- Unit tests for `rawTunnel` (bidirectional TCP copy, connection cleanup)

## Context

Brownfield Go project. Codebase mapped 2026-02-24. Test pattern: `package proxy_test`, stdlib `testing` only, `httptest` for servers, existing helper `startTestProxy` in `mitm_h2_integration_test.go`. `handleForward` and `rawTunnel` are unexported functions in `internal/proxy/proxy.go` — tests must exercise them through the exported `ServeHTTP` surface.

## Constraints

- **Tech stack**: Go stdlib `testing` + `net/http/httptest` only — no new dependencies
- **Scope**: New test file `internal/proxy/proxy_test.go`; no changes to production code

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Config triple-read fixed (v1.0) | Single `os.ReadFile`, shared `[]byte` | ✓ Good |
| `config.Load` signature unchanged (v1.0) | Callers need no update | ✓ Good |
| Test via `ServeHTTP` (v1.1) | `handleForward`/`rawTunnel` are unexported; test through public surface | ✓ Complete |
| Bool overrides only when explicitly set (v1.2) | Prevent CLI defaults from silently overriding YAML | ✓ Complete |

---
*Last updated: 2026-02-24 after milestone v1.2 implementation*
