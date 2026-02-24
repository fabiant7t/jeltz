# State: jeltz

**Last updated:** 2026-02-24

---

## Project Reference

**Core value:** Intercept and modify HTTPS traffic transparently — any rule change takes effect without touching the browser or OS trust store again.

**Current focus:** Milestone v1.7 — add direct tests for subcommand output paths (`ca-path`, `ca-p12-path`, `ca-install-hint`) and banner stability.

---

## Current Position

**Phase:** Milestone transition
**Plan:** v1.7 CLI output test coverage
**Status:** v1.6 complete; next milestone defined
**Last activity:** 2026-02-24 — Added explicit subcommand parsing with unknown-subcommand error handling + parsing tests

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

### Active Constraints

- Preserve existing CLI output formats while adding coverage for subcommand/banner flows
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
- [ ] Plan Phase 6
- [ ] Execute Phase 6

---

*Initialized: 2026-02-24*
