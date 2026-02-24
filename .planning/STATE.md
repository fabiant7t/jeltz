# State: jeltz

**Last updated:** 2026-02-24

---

## Project Reference

**Core value:** Intercept and modify HTTPS traffic transparently — any rule change takes effect without touching the browser or OS trust store again.

**Current focus:** Milestone v1.9 — remaining runtime reliability gaps (`map_local` startup validation, request body limits).

---

## Current Position

**Phase:** Milestone transition
**Plan:** v1.9 reliability follow-ups
**Status:** v1.8 complete; next milestone defined
**Last activity:** 2026-02-24 — Refactored `rawTunnel` sync from done-channel counting to `sync.WaitGroup`

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
| Explicit subcommand parsing and validation | Prevent typo fallback from unintentionally starting proxy |
| CLI output and banner flows covered by tests | Guard user-facing output contracts against regressions |
| rawTunnel synchronization uses WaitGroup | Improve maintainability without changing tunnel behavior |

### Active Constraints

- Preserve current CLI/proxy behavior while reducing remaining maintenance risk
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
- [x] Plan Phase 5
- [x] Execute Phase 5
- [x] Plan Phase 6
- [x] Execute Phase 6
- [x] Plan Phase 7
- [x] Execute Phase 7
- [ ] Plan Phase 8
- [ ] Execute Phase 8

---

*Initialized: 2026-02-24*
