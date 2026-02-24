# Concerns & Technical Debt

**Analysis Date:** 2026-02-24

---

## Critical Issues

**Hardcoded PKCS#12 password published in plain text:** *(known, accepted)*
- The P12 bundle password `"jeltz"` is a compile-time constant in `internal/ca/ca.go` (`P12Password = "jeltz"`) and is printed on the startup banner in `cmd/jeltz/banner.go` (`dim("(password: "+ca.P12Password+")")`). It is also documented verbatim in README.md.
- **Decision:** Intentional. The password is a convenience for importing the CA into browsers/OS trust stores. Security relies on filesystem permissions protecting `ca.p12`, not the password. No action needed.

---

## Technical Debt

**Config loading reads the YAML file twice:**
- `internal/config/config.go` lines 120–161: When a config file is present, it is read once by viper (`v.ReadInConfig()`), once again explicitly for strict YAML validation (`os.ReadFile` + `yaml.NewDecoder`), and then unmarshalled a third time to extract typed rules (`yaml.Unmarshal`). This is fragile — a race or file change between reads could produce inconsistent state, and the triple-parse is unnecessary complexity.
- Impact: Subtle staleness bugs if the file changes mid-startup; maintenance overhead.
- Mitigation path: Read the file once into memory; feed that single byte slice to both viper and yaml.v3.

---

## Missing Pieces

---

## Risks

**Leaf certificate cache size is fixed to 1024 entries (LRU):**
- `internal/ca/ca.go`: Leaf certs are cached in-memory only with a fixed cap (`leafCacheMaxEntries = 1024`). Evicted hosts require re-issuance on next request.
- Risk: Low for development usage, but frequent host churn can increase CPU from repeated leaf issuance.

**Per-host CA locking still serializes same-host issuance:**
- `internal/ca/ca.go`: Leaf issuance now uses per-host locks, so different hosts can issue in parallel. Requests for the same host still serialize during issuance and cache insert.
- Risk: Low for development use; hot single-host bursts still queue on first-miss issuance.
- Mitigation path: Keep as-is unless profiling shows this path is a bottleneck.

**No rate-limiting or connection cap on the proxy listener:**
- `internal/proxy/proxy.go`: The `http.Server` has no maximum connections setting. A client could open many simultaneous CONNECT tunnels, each spawning goroutines and issuing leaf certs.
- Risk: Low for a single-developer localhost tool; relevant if exposed beyond loopback.

---

## Opportunities

**Replace triple-read config parsing with a single-pass approach:**
- Reading the YAML once into `[]byte`, using that for viper initialisation, strict validation, and rule parsing would eliminate the redundant reads and the associated fragility. See `internal/config/config.go`.

**Windows build target is missing from goreleaser:**
- `.goreleaser.yaml` builds only `linux` and `darwin`. Adding `windows` would broaden the tool's reach without code changes.

---

*Concerns audit: 2026-02-24*
