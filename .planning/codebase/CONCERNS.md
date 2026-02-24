# Codebase Concerns

**Analysis Date:** 2026-02-24

## Tech Debt

**State and roadmap documents are stale vs implemented runtime details:**
- Issue: planning references still mention old details (for example viper-based config note in state history and current focus wording), while runtime now uses direct YAML/env parsing and phase checklist is complete.
- Files: `.planning/STATE.md`, `.planning/PROJECT.md`, `.planning/ROADMAP.md`, `internal/config/config.go`
- Impact: future planning automation can prioritize outdated work.
- Fix approach: keep planning docs synchronized after each technical milestone; refresh "Last activity" and decision table entries to match latest commits.

## Known Bugs

## Security Considerations

**Fixed PKCS#12 password is intentionally non-secret:**
- Risk: anyone with filesystem access to `ca.p12` also knows password `jeltz`.
- Files: `internal/ca/ca.go`, `cmd/jeltz/banner.go`, `README.md`
- Current mitigation: design assumes local dev tool; filesystem permissions protect artifacts.
- Recommendations: keep as accepted tradeoff for dev scope; add explicit warning that P12 password is convenience-only.

**Optional upstream TLS verification bypass:**
- Risk: `-insecure-upstream` disables upstream TLS cert verification, enabling MITM of upstream connection.
- Files: `cmd/jeltz/main.go`, `internal/proxy/pipeline.go`
- Current mitigation: opt-in flag only; default is secure verification.
- Recommendations: preserve opt-in behavior; keep banner/log hint when enabled.

## Performance Bottlenecks

**Leaf certificate issuance is CPU-heavy on cache miss:**
- Problem: each miss generates RSA-3072 keypair before caching.
- Files: `internal/ca/ca.go`, `pkg/ca/ca.go`
- Cause: asymmetric key generation cost; no pre-generation pool.
- Improvement path: current 1024-entry LRU cache already mitigates repeat hosts; only optimize further if profiling shows miss-heavy workloads.

## Fragile Areas

**MITM integration flow has high protocol complexity:**
- Files: `internal/proxy/mitm.go`, `internal/proxy/mitm_h2_integration_test.go`
- Why fragile: combines CONNECT hijack, dynamic leaf certs, ALPN negotiation, HTTP/2 ServeConn, and HTTP/1.1 fallback.
- Safe modification: preserve ALPN branch behavior and context propagation; make small changes with integration tests run.
- Test coverage: good integration coverage exists, but relies on socket permissions and can be environment-sensitive.

## Scaling Limits

**Leaf cert cache memory/capacity is fixed by design:**
- Current capacity: 1024 host entries in-memory (`leafCacheMaxEntries`).
- Files: `internal/ca/ca.go`
- Limit: high host churn triggers reissuance CPU overhead after eviction.
- Scaling path: adjust `leafCacheMaxEntries` or make cache size configurable if usage model changes from dev-local patterns.

## Dependencies at Risk

**No high-risk abandoned runtime dependency detected:**
- Risk: Not detected from current dependency set.
- Impact: Not applicable.
- Migration plan: continue minimizing dependency surface (current stack is mostly stdlib + 2 active modules).

## Missing Critical Features

**None for declared local-development scope:**
- Problem: no critical missing feature blocks intended dev MITM proxy usage.
- Blocks: Not applicable.

## Test Coverage Gaps

---

*Concerns audit: 2026-02-24*
