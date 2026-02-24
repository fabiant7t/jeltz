# State: jeltz

**Last updated:** 2026-02-24

---

## Project Reference

**Core value:** Intercept and modify HTTPS traffic transparently — any rule change takes effect without touching the browser or OS trust store again.

**Current focus:** Milestone v1.2 — CLI override semantics (`-insecure-upstream`, `-dump-traffic`, `-max-body-bytes` explicit-only overrides).

---

## Current Position

**Phase:** Implementation complete
**Plan:** v1.2 CLI override semantics
**Status:** Verifying and documenting
**Last activity:** 2026-02-24 — Implemented explicit-only bool/int flag override helpers in `cmd/jeltz/main.go` + tests

---

## Accumulated Context

### Key Decisions

| Decision | Rationale |
|----------|-----------|
| Config triple-read fixed (v1.0) | Single `os.ReadFile`, shared `[]byte` via `v.ReadConfig(bytes.NewReader(rawYAML))` |
| `config.Load` signature unchanged | Callers in `cmd/jeltz/main.go` need no update |
| Test via `ServeHTTP` surface | `handleForward`/`rawTunnel` are unexported; tests must drive them through the exported handler |
| CLI bool/int pointers only when flags are visited | Prevent default CLI values from overriding YAML unexpectedly |

### Active Constraints

- Preserve existing CLI behavior except precedence bug fix
- Keep tests in stdlib `testing` only

### Blockers

None.

### Todos

- [x] Plan Phase 2
- [x] Execute Phase 2

---

*Initialized: 2026-02-24*
