# State: jeltz

**Last updated:** 2026-02-24

---

## Project Reference

**Core value:** Intercept and modify HTTPS traffic transparently — any rule change takes effect without touching the browser or OS trust store again.

**Current focus:** Milestone v1.6 — harden CLI subcommand dispatch behavior.

---

## Current Position

**Phase:** Milestone transition
**Plan:** v1.6 subcommand dispatch hardening
**Status:** v1.5 complete; next milestone defined
**Last activity:** 2026-02-24 — Switched `map_local` serving to streaming file handles + added large-file regression test

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
| Dump traffic body logging now streams | Preserve full upstream response body while capturing snippet |
| map_local serving streams file bodies | Reduce memory footprint on large local responses |

### Active Constraints

- Preserve CLI UX and existing subcommand semantics while hardening unknown-command behavior
- Keep tests in stdlib `testing` only

### Blockers

None.

### Todos

- [x] Plan Phase 2
- [x] Execute Phase 2
- [x] Plan Phase 3
- [x] Execute Phase 3
- [x] Plan Phase 4
- [x] Execute Phase 4
- [ ] Plan Phase 5
- [ ] Execute Phase 5

---

*Initialized: 2026-02-24*
