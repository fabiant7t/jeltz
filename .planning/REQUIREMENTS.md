# Requirements: jeltz

**Defined:** 2026-02-24
**Core Value:** Intercept and modify HTTPS traffic transparently — any rule change takes effect without touching the browser or OS trust store again.

## v1 Requirements

### Config Loading

- [x] **CONF-01**: The YAML config file is read from disk exactly once on startup; the resulting `[]byte` is passed to Viper, the strict `KnownFields` validator, and the rule struct parser — no second or third `os.ReadFile` or `os.Open` call occurs

## v2 Requirements

*(None identified for this cycle)*

## Out of Scope

| Feature | Reason |
|---------|--------|
| Upstream transport timeouts | Not requested for this cycle |
| `map_local` streaming via `http.ServeContent` | Not requested for this cycle |
| `-dump-traffic` truncation fix via `io.TeeReader` | Not requested for this cycle |
| Per-host cert cache eviction | Not requested for this cycle |
| Windows build target | Not requested for this cycle |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| CONF-01 | Phase 1 | Complete |

**Coverage:**
- v1 requirements: 1 total
- Mapped to phases: 1
- Unmapped: 0 ✓

---
*Requirements defined: 2026-02-24*
*Last updated: 2026-02-24 after initial definition*
