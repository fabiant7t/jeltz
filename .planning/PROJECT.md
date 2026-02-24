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
- [x] Upstream transport timeout configuration added (dial/TLS handshake/response header/idle connection)

### Out of Scope

- `map_local` streaming via `http.ServeContent` — not requested
- `io.TeeReader` fix for `-dump-traffic` truncation — not requested
- Per-host cert cache eviction — not requested
- Windows build target — not requested

## Current Milestone: v1.3 — Dump Traffic Body Streaming

**Goal:** Remove response truncation when `-dump-traffic` is enabled by streaming upstream bodies while logging snippets.

**Target features:**
- Replace `dumpBody` read-all buffering path with streaming (`io.TeeReader` or equivalent)
- Ensure clients receive full upstream response bodies even when traffic dump is enabled
- Add regression test covering large response body with dump enabled

## Completed Milestone: v1.1 — Proxy Handler Tests

**Goal:** Close the test coverage gap on the two untested proxy handlers in `internal/proxy/proxy.go`.

**Target features:**
- Unit tests for `handleForward` (no-pipeline fallback + pipeline path)
- Unit tests for `rawTunnel` (bidirectional TCP copy, connection cleanup)

## Completed Milestone: v1.2 — CLI Override Semantics

**Goal:** Ensure boolean CLI flags do not silently override YAML defaults unless explicitly passed.

**Target features:**
- `cmd/jeltz/main.go` only sets `CLIOverrides` bool pointers when corresponding flags were visited
- Unit tests for explicit-vs-implicit flag override behavior in `cmd/jeltz/main_test.go`

## Completed Milestone: v1.3 — Upstream Transport Timeouts

**Goal:** Prevent indefinite upstream stalls by adding explicit transport timeouts with config + CLI control.

**Target features:**
- `internal/proxy/pipeline.go` transport now includes dial/TLS handshake/response header/idle timeouts
- New config keys + CLI flags for timeout tuning
- Tests for timeout defaults/overrides and response-header-timeout behavior

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
| Upstream transport timeouts configurable (v1.3) | Prevent indefinite blocking on stalled upstream | ✓ Complete |

---
*Last updated: 2026-02-24 after milestone v1.3 implementation*
