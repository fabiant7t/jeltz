# Concerns & Technical Debt

**Analysis Date:** 2026-02-24

---

## Critical Issues

**Hardcoded PKCS#12 password published in plain text:** *(known, accepted)*
- The P12 bundle password `"jeltz"` is a compile-time constant in `internal/ca/ca.go` (`P12Password = "jeltz"`) and is printed on the startup banner in `cmd/jeltz/banner.go` (`dim("(password: "+ca.P12Password+")")`). It is also documented verbatim in README.md.
- **Decision:** Intentional. The password is a convenience for importing the CA into browsers/OS trust stores. Security relies on filesystem permissions protecting `ca.p12`, not the password. No action needed.

---

## Technical Debt

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

**No rate-limiting or connection cap on the proxy listener:** *(known, accepted)*
- `internal/proxy/proxy.go`: The `http.Server` has no maximum connections setting. A client could open many simultaneous CONNECT tunnels, each spawning goroutines and issuing leaf certs.
- **Decision:** Intentional for local development usage. This is not treated as backlog work unless deployment scope changes beyond a dev-only tool.

---

## Opportunities

---

*Concerns audit: 2026-02-24*
