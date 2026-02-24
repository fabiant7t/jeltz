# Concerns & Technical Debt

**Analysis Date:** 2026-02-24

---

## Critical Issues

**Hardcoded PKCS#12 password published in plain text:** *(known, accepted)*
- The P12 bundle password `"jeltz"` is a compile-time constant in `internal/ca/ca.go` (`P12Password = "jeltz"`) and is printed on the startup banner in `cmd/jeltz/banner.go` (`dim("(password: "+ca.P12Password+")")`). It is also documented verbatim in README.md.
- **Decision:** Intentional. The password is a convenience for importing the CA into browsers/OS trust stores. Security relies on filesystem permissions protecting `ca.p12`, not the password. No action needed.

**Leaf certificates never expire in practice (100-year validity):**
- Both the CA certificate and all per-host leaf certificates are issued with `validity = 100 * 365 * 24 * time.Hour` (`internal/ca/ca.go`). Leaf certs use 2048-bit RSA keys which may be considered weak before that expiry.
- Impact: Compromised leaf keys remain usable for 100 years; browsers may reject 100-year certificates in future policy updates.
- Mitigation path: Reduce leaf validity to 1–2 years and implement automatic re-issuance on expiry.

**Leaf certs use only 2048-bit RSA keys:**
- `internal/ca/ca.go` line 170: `pkgca.IssueLeaf(ca.key, ca.cert, host, 2048, validity)`. The root CA uses 3072 bits, but each issued leaf cert uses 2048 bits.
- Impact: Weaker than the CA, inconsistent with modern recommendations (NIST recommends 3072+ after 2030).
- Mitigation path: Increase leaf key size to 3072 or switch to ECDSA P-256/P-384.

---

## Technical Debt

**Config loading reads the YAML file twice:**
- `internal/config/config.go` lines 120–161: When a config file is present, it is read once by viper (`v.ReadInConfig()`), once again explicitly for strict YAML validation (`os.ReadFile` + `yaml.NewDecoder`), and then unmarshalled a third time to extract typed rules (`yaml.Unmarshal`). This is fragile — a race or file change between reads could produce inconsistent state, and the triple-parse is unnecessary complexity.
- Impact: Subtle staleness bugs if the file changes mid-startup; maintenance overhead.
- Mitigation path: Read the file once into memory; feed that single byte slice to both viper and yaml.v3.

**Subcommand dispatch via manual `os.Args` slice check:**
- `cmd/jeltz/main.go` lines 29–41: Subcommands (`ca-path`, `ca-p12-path`, `ca-install-hint`) are handled by a bare `switch os.Args[1]` check before the `flag.FlagSet` is even created. Any unknown argument falls through to the main proxy flow and is silently ignored by `flag.ExitOnError`.
- Impact: Typos in subcommand names silently start the proxy instead of returning an error; `--help` on a subcommand is not handled.
- Mitigation path: Use a proper command dispatcher (e.g., `flag.NewFlagSet` per subcommand, or a lightweight CLI library).

**`rawTunnel` (plain TCP fallback in `internal/proxy/proxy.go`) only waits for one goroutine to finish:**
- Lines 125–137: `done` channel has capacity 2, but `<-done` is called twice after launching two goroutines. This is correct, but the pattern is unusual and any reader must verify the channel capacity matches the goroutine count. A future refactor changing the capacity would silently break cleanup.
- Impact: Low risk now, but fragile under maintenance.
- Mitigation path: Use `sync.WaitGroup` for clarity.

---

## Missing Pieces

**No test for the startup banner or subcommand flows:**
- `cmd/jeltz/banner.go` and `cmd/jeltz/main.go` subcommand functions (`runCAPath`, `runCAP12Path`, `runCAInstallHint`) have no tests.
- Risk: Silent breakage of `ca-install-hint` output format on platforms; banner format regressions.

**No test coverage for `pkg/xdg` on non-Linux platforms:**
- `pkg/xdg/xdg.go` contains XDG path resolution logic. Tests exist but run on the current platform only; behavior on macOS or Windows is untested.
- Risk: Wrong data directories on macOS/Windows users.

**No request body size limit for upstream forwarding:**
- `internal/proxy/pipeline.go`: Request bodies are forwarded as-is to upstream with no size cap. An attacker sending a very large POST body through the proxy could exhaust memory or disk if the upstream is slow.
- Impact: Low risk for a localhost developer tool, but worth noting for shared-network use.

**`map_local` does not call `os.Stat` on error from disk (returns generic error for missing rule dir):**
- `internal/rules/maplocal.go` lines 103–106: If `os.Stat(r.FSPath)` fails (e.g., the configured rule directory does not exist), the error is wrapped and returned — the pipeline propagates this as a 500. There is no startup-time validation that rule filesystem paths exist.
- Impact: A misconfigured `path` in a map_local rule produces an opaque 500 at request time rather than a clear startup error.
- Mitigation path: Validate that rule paths exist during `rules.Compile`.

---

## Risks

**Leaf certificate disk cache is never pruned:**
- `internal/ca/ca.go`: Leaf certs are written to `~/.local/share/jeltz/certs/<host>.pem` and cached in memory indefinitely. No eviction or expiry check occurs when loading from disk.
- Risk: A long-running instance will accumulate stale, potentially compromised leaf certs on disk and in memory. The memory cache grows unbounded over the process lifetime as new hosts are visited.

**Single global `sync.Mutex` on the CA for all leaf cert issuance:**
- `internal/ca/ca.go` lines 88–116: `ca.mu.Lock()` is held for the entire duration of `LeafCert`, including disk I/O and RSA key generation (for new hosts). Under concurrent HTTPS CONNECT requests to many previously-unseen hosts, all goroutines serialize on this one lock.
- Risk: Latency spike on first connection to many distinct hosts simultaneously (e.g., browser startup loading many resources).
- Mitigation path: Use a per-host lock or generate certs outside the global lock, inserting only under the lock.

**`errTraversal` is compared by value (`==`), not with `errors.Is`:**
- `internal/rules/maplocal.go` line 146: `IsTraversal` does `return err == errTraversal`. If the error is ever wrapped (e.g., `fmt.Errorf("...: %w", errTraversal)`), `IsTraversal` returns false and the traversal attempt falls through as a 500 instead of a 403.
- Files: `internal/rules/maplocal.go`
- Mitigation path: Use a sentinel type and `errors.As`, or define a custom error type.

**No rate-limiting or connection cap on the proxy listener:**
- `internal/proxy/proxy.go`: The `http.Server` has no maximum connections setting. A client could open many simultaneous CONNECT tunnels, each spawning goroutines and issuing leaf certs.
- Risk: Low for a single-developer localhost tool; relevant if exposed beyond loopback.

**`goreleaser` release builds strip debug info (`-s -w` ldflags):**
- `.goreleaser.yaml` line 18: `ldflags: -s -w` strips the symbol table and DWARF debug info from release binaries.
- Risk: Crash reports and stack traces from production builds will be harder to diagnose without separate symbol files.

---

## Opportunities

**Replace triple-read config parsing with a single-pass approach:**
- Reading the YAML once into `[]byte`, using that for viper initialisation, strict validation, and rule parsing would eliminate the redundant reads and the associated fragility. See `internal/config/config.go`.

**Increase leaf cert key size or migrate to ECDSA:**
- Changing `IssueLeaf` in `internal/ca/ca.go` from 2048-bit RSA to ECDSA P-256 would produce smaller, faster, and more future-proof certificates.

**Add per-host lock granularity in the CA:**
- Replacing the global mutex in `internal/ca/ca.go` with a `sync.Map` or a singleflight group per host would allow concurrent cert generation for distinct hosts.

**Windows build target is missing from goreleaser:**
- `.goreleaser.yaml` builds only `linux` and `darwin`. Adding `windows` would broaden the tool's reach without code changes.

---

*Concerns audit: 2026-02-24*
