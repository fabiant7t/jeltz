# State: jeltz

**Last updated:** 2026-02-24

---

## Project Reference

**Core value:** Intercept and modify HTTPS traffic transparently — any rule change takes effect without touching the browser or OS trust store again.

**Current focus:** Milestone v1.1 — tests for `handleForward` and `rawTunnel` proxy handlers.

---

## Current Position

**Phase:** Not started (defining requirements)
**Plan:** —
**Status:** Defining requirements
**Last activity:** 2026-02-24 — Milestone v1.1 started

---

## Accumulated Context

### Key Decisions

| Decision | Rationale |
|----------|-----------|
| Config triple-read fixed (v1.0) | Single `os.ReadFile`, shared `[]byte` via `v.ReadConfig(bytes.NewReader(rawYAML))` |
| `config.Load` signature unchanged | Callers in `cmd/jeltz/main.go` need no update |
| Test via `ServeHTTP` surface | `handleForward`/`rawTunnel` are unexported; tests must drive them through the exported handler |

### Active Constraints

- Go stdlib `testing` + `net/http/httptest` only — no new dependencies
- New file: `internal/proxy/proxy_test.go`
- No changes to production code

### Blockers

None.

### Todos

- [ ] Plan Phase 2
- [ ] Execute Phase 2

---

*Initialized: 2026-02-24*
