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

**`map_local` reads entire files into memory with `os.ReadFile`:**
- `internal/proxy/pipeline.go` line 173: `data, err := os.ReadFile(mlr.FSTarget)`. The entire file is loaded into a `bytes.Reader` before any response is written.
- Impact: Large mock files (multi-MB assets, videos) will spike memory use proportional to the number of concurrent requests. No streaming path exists.
- Mitigation path: Use `http.ServeContent` or open a `*os.File` and stream it; fall back to ReadFile only for content-sniffing when extension detection fails.

**`dumpBody` in `internal/proxy/pipeline.go` consumes and buffers the upstream response body:**
- Lines 270–283: `io.ReadAll` up to `maxBodyBytes` is called, the original body is closed, and a `bytes.Reader` wrapping the consumed bytes is returned. Beyond `maxBodyBytes`, bytes are silently dropped — the caller receives a truncated body.
- Impact: When `-dump-traffic` is enabled on a large upstream response, the client receives only the first `maxBodyBytes` bytes with no indication that truncation occurred. No `Content-Length` correction is applied.
- Mitigation path: Use `io.TeeReader` to log the snippet while streaming the full body through to the client.

**Subcommand dispatch via manual `os.Args` slice check:**
- `cmd/jeltz/main.go` lines 29–41: Subcommands (`ca-path`, `ca-p12-path`, `ca-install-hint`) are handled by a bare `switch os.Args[1]` check before the `flag.FlagSet` is even created. Any unknown argument falls through to the main proxy flow and is silently ignored by `flag.ExitOnError`.
- Impact: Typos in subcommand names silently start the proxy instead of returning an error; `--help` on a subcommand is not handled.
- Mitigation path: Use a proper command dispatcher (e.g., `flag.NewFlagSet` per subcommand, or a lightweight CLI library).

**`rawTunnel` (plain TCP fallback in `internal/proxy/proxy.go`) only waits for one goroutine to finish:**
- Lines 125–137: `done` channel has capacity 2, but `<-done` is called twice after launching two goroutines. This is correct, but the pattern is unusual and any reader must verify the channel capacity matches the goroutine count. A future refactor changing the capacity would silently break cleanup.
- Impact: Low risk now, but fragile under maintenance.
- Mitigation path: Use `sync.WaitGroup` for clarity.

**`insecureUpstream` flag is always passed as a pointer to `CLIOverrides` even when unset:**
- `cmd/jeltz/main.go` lines 66, 80–86: The `-insecure-upstream` boolean flag is always stored in `cli.InsecureUpstream` (a `*bool`), so `config.Load` always sees it as set and the config-file value can never win. This is by design for booleans (Go flags default to `false`), but it means a user cannot set `insecure_upstream: true` in the YAML and have it honored without also passing `-insecure-upstream` on the CLI — the flag default (`false`) silently overrides it.
- Files: `cmd/jeltz/main.go`, `internal/config/config.go`
- Impact: Config-file-set `insecure_upstream: true` is silently overridden to `false` at runtime.
- Mitigation path: Only set `cli.InsecureUpstream` when the flag was explicitly provided (use `flag.Visit`).

---

## Missing Pieces

**No test coverage for `proxy.ServeHTTP` / `handleForward` end-to-end:**
- `internal/proxy/proxy.go` `handleForward` (lines 141–222) has no dedicated test. The fallback path (no pipeline configured) and the pipeline-integrated path for plain HTTP forward requests are exercised only indirectly through higher-level integration tests.
- Risk: Regressions in HTTP/1.1 plain proxy forwarding could go undetected.

**No test coverage for `rawTunnel` (non-MITM CONNECT fallback):**
- `internal/proxy/proxy.go` lines 106–138: The raw TCP tunnel path (triggered when no CA is configured) has no test.
- Risk: The tunnel's bidirectional copy logic could silently break.

**No test for the startup banner or subcommand flows:**
- `cmd/jeltz/banner.go` and `cmd/jeltz/main.go` subcommand functions (`runCAPath`, `runCAP12Path`, `runCAInstallHint`) have no tests.
- Risk: Silent breakage of `ca-install-hint` output format on platforms; banner format regressions.

**No test coverage for `pkg/xdg` on non-Linux platforms:**
- `pkg/xdg/xdg.go` contains XDG path resolution logic. Tests exist but run on the current platform only; behavior on macOS or Windows is untested.
- Risk: Wrong data directories on macOS/Windows users.

**No connection timeout on the upstream `http.Transport` in `Pipeline`:**
- `internal/proxy/pipeline.go` lines 72–78: `http.Transport` is created with only `InsecureSkipVerify` set. No `DialContext` with timeout, no `TLSHandshakeTimeout`, no `ResponseHeaderTimeout` are configured. The transport falls back to Go's defaults (no dial timeout, no header timeout).
- Impact: A stalled upstream server will hold a proxy goroutine open indefinitely.
- Mitigation path: Set explicit timeouts on the transport.

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

**Add upstream transport timeouts:**
- Configuring `DialContext` (10 s), `TLSHandshakeTimeout` (10 s), and `ResponseHeaderTimeout` (30 s) on the transport in `internal/proxy/pipeline.go` would prevent goroutine leaks from stalled upstream connections.

**Stream `map_local` responses instead of buffering:**
- Replacing `os.ReadFile` + `bytes.NewReader` with `http.ServeContent` in `internal/proxy/pipeline.go` would give free Range request support, correct `Last-Modified`/`ETag` handling, and avoid full-file buffering.

**Switch `dumpBody` to `io.TeeReader`:**
- Replacing the read-all-and-wrap pattern in `internal/proxy/pipeline.go` with a `TeeReader` that writes a snippet to a buffer while streaming the original body would fix silent response truncation when `-dump-traffic` is used.

**Increase leaf cert key size or migrate to ECDSA:**
- Changing `IssueLeaf` in `internal/ca/ca.go` from 2048-bit RSA to ECDSA P-256 would produce smaller, faster, and more future-proof certificates.

**Add per-host lock granularity in the CA:**
- Replacing the global mutex in `internal/ca/ca.go` with a `sync.Map` or a singleflight group per host would allow concurrent cert generation for distinct hosts.

**Windows build target is missing from goreleaser:**
- `.goreleaser.yaml` builds only `linux` and `darwin`. Adding `windows` would broaden the tool's reach without code changes.

---

*Concerns audit: 2026-02-24*
