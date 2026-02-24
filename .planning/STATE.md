# State: jeltz

**Last updated:** 2026-02-24

---

## Project Reference

**Core value:** Intercept and modify HTTPS traffic transparently — any rule change takes effect without touching the browser or OS trust store again.

**Current focus:** Milestone v1.4 — fix dump-body truncation when `-dump-traffic` is enabled.

---

## Current Position

**Phase:** Milestone transition
**Plan:** v1.4 dump-body streaming
**Status:** v1.3 complete; next milestone defined
**Last activity:** 2026-02-24 — Added upstream transport timeout config/flags + pipeline timeout behavior tests

---

## Accumulated Context

### Key Decisions

| Decision | Rationale |
|----------|-----------|
| Config triple-read fixed (v1.0) | Single `os.ReadFile`, shared `[]byte` via `v.ReadConfig(bytes.NewReader(rawYAML))` |
| `config.Load` signature unchanged | Callers in `cmd/jeltz/main.go` need no update |
| Test via `ServeHTTP` surface | `handleForward`/`rawTunnel` are unexported; tests must drive them through the exported handler |
| CLI bool/int pointers only when flags are visited | Prevent default CLI values from overriding YAML unexpectedly |
| Upstream transport timeout keys added | Bound dial/handshake/header/idle wait times for upstream requests |

### Active Constraints

- Preserve response semantics while fixing dump-body truncation
- Keep tests in stdlib `testing` only

### Blockers

None.

### Todos

- [x] Plan Phase 2
- [x] Execute Phase 2
- [ ] Plan Phase 3
- [ ] Execute Phase 3

---

*Initialized: 2026-02-24*
