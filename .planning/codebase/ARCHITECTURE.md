# Architecture

**Analysis Date:** 2026-02-24

## Pattern

Single-binary CLI tool implementing a TLS-intercepting HTTP/HTTPS forward proxy (MITM proxy). The process runs as a long-lived server, intercepting traffic to apply configurable rules before forwarding to upstream hosts. No microservices, no external runtime dependencies — all state is on-disk under the XDG data directory.

## Core Components

**Proxy Server (`internal/proxy/proxy.go`):**
- Implements `http.Handler` via `ServeHTTP`
- Dispatches on HTTP method: `CONNECT` requests go through the MITM handler, all other methods go through the plain HTTP forward handler
- Owns a `*Pipeline` (rule engine + transport) and a `caLoader` (certificate authority)
- Handles graceful shutdown via `context.Context` cancel on `SIGINT`/`SIGTERM`

**MITM Handler (`internal/proxy/mitm.go`):**
- Hijacks the TCP connection after receiving HTTP CONNECT
- Performs TLS handshake as a TLS server, offering ALPN `h2` and `http/1.1`
- Issues per-host leaf certificates on demand via `caLoader.LeafCert(host)`
- Dispatches to `serveH2` (uses `golang.org/x/net/http2`) or `serveHTTP1` (manual read loop) depending on negotiated protocol
- Both paths build a `FlowContext` and call `pipeline.Run`

**Pipeline (`internal/proxy/pipeline.go`):**
- Executes the request/response processing chain for every intercepted request:
  1. Build `FlowMeta` from `FlowContext`
  2. Apply matching request header rules (`RuleSet.Headers`)
  3. Resolve map-local rules (first match wins); if matched, serve local file
  4. If no map-local match, perform upstream round-trip via shared `*http.Transport`
  5. Apply matching response header rules
  6. Apply per-rule response ops from the matched map-local rule (if any)
  7. Optionally dump traffic headers/body at debug level
- `FlowContext` carries mutable request state (headers, body, metadata)
- `ResponseResult` carries the response (status, headers, body reader, source tag)

**Rule Engine (`internal/rules/`):**
- `rules.go`: `FlowMeta` struct; `Match` (compiled host + path regexes + HTTP method set); `Matches()` evaluation
- `ruleset.go`: `RuleSet` holding `[]*HeaderRule` and `[]*MapLocalRule`; `Compile()` takes raw config and produces the compiled rule set
- `headers.go`: `Ops` (ordered delete + set operations on `http.Header`); `CompileOps()` and `Apply()`
- `maplocal.go`: `MapLocalRule` with prefix-stripping URL-to-filesystem resolution; traversal protection; MIME detection via extension + sniffing

**Certificate Authority (`internal/ca/ca.go`):**
- Loads or generates a root CA (RSA 3072, 100-year validity) on first run from `<data-dir>/ca.key.pem` + `ca.crt.pem`
- Issues and caches per-host leaf certificates (RSA 2048, 100-year validity) in memory and on disk at `<data-dir>/certs/<host>.pem`
- Writes a PKCS#12 bundle (`ca.p12`, fixed password `jeltz`) for browser/OS import
- Thread-safe via `sync.Mutex` around the in-memory cert cache

**Config (`internal/config/config.go`):**
- Loads from YAML file (via Viper) with strict field validation via `yaml.v3` `KnownFields(true)`
- Supports `JELTZ_*` environment variable overrides (auto env, `_` separator)
- CLI flags are the highest-precedence override layer via `CLIOverrides`
- `BasePath` and `DataDir` are resolved against XDG directories when relative or empty

**Supporting packages:**
- `internal/logging/logging.go`: `slog.Logger` factory; stable structured log key constants
- `internal/httpx/hopbyhop.go`: `RemoveHopByHop()` strips connection-scoped headers from forwarded requests/responses
- `pkg/ca/ca.go`: Pure crypto primitives — `GenerateCA` and `IssueLeaf` (no disk I/O)
- `pkg/p12/p12.go`: Pure-Go PKCS#12 encoder (PFX v3, PBE-SHA1-3DES, HMAC-SHA1 MAC); no third-party crypto dependencies
- `pkg/xdg/xdg.go`: XDG Base Directory resolution for config and data dirs

## Data Flow

**HTTPS request (CONNECT + MITM):**

1. Client sends `CONNECT host:port HTTP/1.1`
2. `proxy.ServeHTTP` → `handleCONNECT` → TCP connection hijacked
3. `200 Connection Established` written to client
4. TLS handshake: `ca.LeafCert(host)` issues/retrieves per-host cert; ALPN negotiates `h2` or `http/1.1`
5. Negotiated protocol dispatches to `serveH2` or `serveHTTP1`
6. Each request is wrapped in a `FlowContext` and passed to `pipeline.Run`
7. Pipeline applies request header ops, checks map-local rules, then either serves local file or round-trips to upstream
8. Pipeline applies response header ops
9. Response is written back over the TLS connection to the client

**HTTP request (plain forward):**

1. Client sends a plain HTTP request with absolute URL
2. `proxy.ServeHTTP` → `handleForward`
3. If pipeline configured: builds `FlowContext`, calls `pipeline.Run`, writes result via `WriteResponse`
4. Fallback (no pipeline): direct `http.DefaultTransport.RoundTrip`

**Configuration load (startup):**

1. XDG dirs resolved for config and data paths
2. `config.Load`: YAML file read + Viper defaults + env vars + CLI overrides merged
3. `ca.Load`: CA key/cert loaded from disk, generated on first run, P12 written if missing
4. `rules.Compile`: YAML raw rules compiled to typed `RuleSet` (regexes pre-compiled, paths resolved)
5. `proxy.New` + `pipeline.NewPipeline` constructed; server starts listening

## Key Design Decisions

- **Two-layer package split**: `internal/` packages are application-specific (use config types, slog, etc.); `pkg/` packages are pure, reusable primitives with no internal imports. This makes `pkg/ca` and `pkg/p12` independently testable and reusable.
- **No third-party crypto**: PKCS#12 encoding is implemented from scratch in `pkg/p12` using only standard library crypto, avoiding large dependency chains.
- **Compiled rules at startup**: All regexes and paths in the rule set are compiled once at startup, not per-request. Rule application is a linear scan through `RuleSet.Headers` and `RuleSet.MapLocal` slices.
- **First-match-wins for map-local**: `Pipeline.Run` breaks on the first matching `MapLocalRule`. Header rules always apply all matches.
- **Shared transport with per-pipeline TLS config**: `Pipeline` holds a single `*http.Transport` for connection pooling to upstream, configured with `InsecureSkipVerify` option at construction time.
- **Interface-based CA**: `proxy.go` uses the `caLoader` interface rather than `*ca.CA` directly, enabling mock substitution in tests.
- **Graceful shutdown**: `signal.NotifyContext` propagates `SIGINT`/`SIGTERM` to `srv.Shutdown` with a 5-second timeout.
- **Structured logging only**: All logging goes through `*slog.Logger` with stable key constants from `internal/logging`. No `fmt.Println` in hot paths.
- **No CGO**: `CGO_ENABLED=0` is set in build configuration, producing fully static binaries.

## Entry Points

**Main proxy server:**
- `cmd/jeltz/main.go`: `func main()` — parses flags, loads config, loads CA, compiles rules, starts `proxy.Server`

**Subcommands (inline in `main()`):**
- `jeltz ca-path` → `runCAPath()` — prints path of CA cert PEM file
- `jeltz ca-p12-path` → `runCAP12Path()` — prints path of CA PKCS#12 bundle
- `jeltz ca-install-hint` → `runCAInstallHint()` — prints per-platform CA trust installation instructions

**HTTP dispatch surface (runtime):**
- `proxy.Server.ServeHTTP` — all incoming HTTP/1.1 connections from the configured listen address
- No REST API, no webhooks, no gRPC. The proxy is purely a TCP listener acting as an HTTP intermediary.

## Error Handling

**Strategy:** Errors propagate upward via `error` return values and are handled at the boundary (main, ServeHTTP, pipeline). Fatal startup errors (`config.Load`, `ca.Load`, `rules.Compile`) call `os.Exit(1)` after logging. In-flight request errors return HTTP error responses (`502`, `403`, `500`) rather than terminating the server.

**Patterns:**
- All errors wrapped with `fmt.Errorf("context: %w", err)` for stack-friendly messages
- `rules.IsTraversal(err)` sentinel for path traversal detection (returns 403 to client)
- Upstream dial failures in `Pipeline.roundtrip` return `emptyResult(502)` without propagating error to avoid crashing the pipeline
- `io.Copy` errors in streaming paths are explicitly suppressed with `//nolint:errcheck` comments

## Cross-Cutting Concerns

**Logging:** `log/slog` with `TextHandler` to stderr. Level configurable (debug/info/warn/error). Key constants defined in `internal/logging/logging.go`. Sensitive headers (`Authorization`, `Cookie`, `Set-Cookie`) are redacted in traffic dumps.

**Validation:** Config is validated twice — once by Viper (for defaults/env), once by `yaml.v3` with `KnownFields(true)` for strict unknown-field rejection. Rule regexes are compiled at startup and fail fast.

**Authentication:** None (this is a local proxy, expected to bind to loopback). No auth on the proxy listener.

---

*Architecture analysis: 2026-02-24*
