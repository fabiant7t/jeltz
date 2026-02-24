SPEC: jeltz — Go HTTPS MITM Proxy with HARD client→proxy HTTP/2, YAML config, XDG paths, slog logging

Program Name
- jeltz (Prostetnic Vogon Jeltz, THGTTG)

Purpose
- A local developer explicit proxy that supports:
  1) HTTP forward proxying (outer proxy listener)
  2) HTTPS interception (MITM) for CONNECT tunnels
  3) Rule-based behavior on requests/responses:
     - map-local: serve local filesystem content for matching requests (strip matched path prefix)
     - header transforms: append, replace, delete (including wildcard delete by value-regex across all names)
     - future: map-remote (not implemented now, but rule model anticipates it)
  4) Portable configuration (XDG-compliant defaults; relative paths resolved via base_path)

Hard Requirements
- Client → proxy MUST support HTTP/2 for intercepted HTTPS traffic:
  - After CONNECT + TLS handshake, jeltz MUST negotiate ALPN "h2" when offered and serve HTTP/2 streams correctly.
  - If a client does not offer "h2", jeltz MAY serve HTTP/1.1 as fallback, but h2 support is mandatory.
- No WebSocket support.

Allowed Dependencies
- gopkg.in/yaml.v3             (YAML decoding/strict validation)
- github.com/spf13/viper       (configuration management)
- golang.org/x/net/http2       (HTTP/2 server on hijacked TLS connections)  <-- REQUIRED

Non-Goals (v1)
- Transparent proxying (iptables/TPROXY)
- Web UI / TUI
- Certificate pinning bypass
- Full RFC support for HTTP/2 proxy semantics on the outer listener (before CONNECT)

HTTP/2 Scope (Important)
- The outer proxy listener (the port you point your browser to as “HTTP proxy”) is HTTP/1.1 in v1.
- The hard HTTP/2 requirement applies to the decrypted “inside” of CONNECT MITM TLS connections:
  - client does CONNECT to jeltz (HTTP/1.1)
  - then client starts TLS to target host through jeltz
  - jeltz terminates TLS and must support HTTP/2 streams over that TLS session if ALPN chooses "h2"

============================================================
Layered Implementation Plan (MUST follow this order)
============================================================

L0 — Skeleton, XDG Paths, CLI, Structured Logging (slog)
0.1 Project layout:
  - /cmd/jeltz/main.go
  - /internal/xdg
  - /internal/logging
  - /internal/config
  - /internal/rules
  - /internal/ca
  - /internal/httpx
  - /internal/proxy
  - /internal/testutil (optional helpers for integration tests)

0.2 XDG directory handling:
  - Config dir:
    - If $XDG_CONFIG_HOME set: $XDG_CONFIG_HOME/jeltz
    - Else: $HOME/.config/jeltz
  - Data dir:
    - If $XDG_DATA_HOME set: $XDG_DATA_HOME/jeltz
    - Else: $HOME/.local/share/jeltz
  - Implement:
    - func ConfigDir() (string, error)
    - func DataDir() (string, error)
  - Ensure dirs exist.

0.3 CLI flags (flag package), CLI MUST override config:
  - --listen              default "127.0.0.1:8080"
  - --config              default "<XDG_CONFIG>/jeltz/config.yaml" if exists else empty
  - --base-path           default "<XDG_CONFIG>/jeltz"
  - --data-dir            default "<XDG_DATA>/jeltz"
  - --log-level           debug|info|warn|error (default info)
  - --insecure-upstream   default false
  - --dump-traffic        default false
  - --max-body-bytes      default 1048576
  - subcommands:
    - `jeltz ca-path`
    - `jeltz ca-install-hint`

0.4 Logging:
  - Use `log/slog` with TextHandler (JSONHandler optional).
  - Ensure stable keys:
    - component, event, client, method, scheme, host, path, status, source, duration_ms, proto, error

Acceptance L0
- `jeltz --help` works.
- Startup logs include resolved XDG dirs and active config file path (if any).

------------------------------------------------------------

L1 — Configuration Management (Viper + YAML) + Strict Validation
1.1 Load config with viper:
  - Precedence:
    1) defaults
    2) config file
    3) env (optional; prefix JELTZ_)
    4) CLI flags
  - Default config:
    - if --config empty: load <XDG_CONFIG>/jeltz/config.yaml if it exists
    - if --config set: must exist; else error

1.2 Config schema version:
  - version: int, required (must equal 1)

1.3 Effective base_path resolution:
  - Let xdgCfg = <XDG_CONFIG>/jeltz
  - If base_path missing/empty/"." => effective_base_path = xdgCfg
  - If base_path relative => resolve against xdgCfg
  - If absolute => use as-is (warn: less portable)

1.4 Effective data_dir:
  - If data_dir empty => XDG data dir
  - If relative => resolve against XDG data dir base (or error; choose one):
    - v1: treat relative data_dir as relative to XDG data dir base directory
  - If absolute => use as-is

1.5 Use yaml.v3 for strict validation:
  - After viper loads raw config, re-decode using yaml.v3 into structs with:
    - KnownFields(true) to reject unknown keys (recommended)
  - Validate all rules and ops (details below)

Acceptance L1
- Unknown YAML fields are rejected with clear errors (file/line if possible).
- Relative paths resolve correctly.

------------------------------------------------------------

L2 — Outer HTTP Proxy Listener (HTTP/1.1)
2.1 Start net/http server at cfg.listen:
  - http.Server{
      Addr: cfg.Listen,
      Handler: ProxyHandler,
      ReadHeaderTimeout: 10s (recommended),
      IdleTimeout: 60s (recommended),
    }

2.2 Routing:
  - CONNECT => CONNECT MITM handler (stub as raw tunnel until L8)
  - Non-CONNECT => forward proxy handler

2.3 Forward proxy (non-CONNECT):
  - Expect absolute-form URL (common for HTTP proxy).
  - Build outbound request with method/url/headers/body (stream).
  - Strip hop-by-hop headers (both directions):
    - Connection, Proxy-Connection, Keep-Alive, TE, Trailer, Transfer-Encoding, Upgrade
  - Use http.Transport with Proxy=nil.

Acceptance L2
- HTTP forwarding works.
- CONNECT stub can temporarily be a raw tunnel.

------------------------------------------------------------

L3 — Rule Engine: Structured Match + Supported Methods
3.1 Supported methods set (validation):
  - GET, HEAD, POST, PUT, DELETE, CONNECT, OPTIONS, TRACE, PATCH

3.2 Flow metadata:
  - Method
  - Scheme ("http" or "https")
  - Host (hostname only, without port)
  - Port (if known)
  - Path (URL.Path, leading slash)
  - RawQuery
  - FullPathWithQuery

3.3 Match object:
  match:
    methods: [ ... ]   # optional; empty => any
    host: "regex"      # required; matches hostname only
    path: "regex"      # required; matches URL.Path only
  - Regex compile at load-time.

3.4 Ordering:
  - Rules are evaluated in file order.
  - Header rules: apply to every matching request/response in order.
  - map-local: first match wins for choosing body source; header rules still apply.

Acceptance L3
- Unit tests for method set includes PUT/DELETE/PATCH/etc.
- Host regex matches without port.

------------------------------------------------------------

L4 — Header Transforms: set + delete + wildcard delete by value-regex
4.1 Operation lists (ORDERED) for determinism:
Ops:
  delete: []HeaderDeleteOp
  set:    []HeaderSetOp

HeaderSetOp:
  name: string
  mode: "replace" | "append"
  value: string

HeaderDeleteOp variants:
(A) name-based:
  name: "Header-Name"
  value_regex: "regex" (optional)
(B) wildcard by value:
  any_name: true
  value_regex: "regex" (required)

4.2 Semantics:
- Apply delete ops first, then set ops.
- Delete by name:
  - without value_regex => remove all values (Header.Del)
  - with value_regex => remove only matching values; preserve remaining values order
- Wildcard delete:
  - iterate all headers and values; remove any values matching value_regex regardless of header name
- Case-insensitive header names; net/http canonicalization used internally.

4.3 Implementation details:
- For selective deletion by value:
  - values := Header.Values(name)
  - keep := filter(values, not matches regex)
  - Header.Del(name)
  - for v in keep: Header.Add(name, v)
- For wildcard delete:
  - iterate over map keys snapshot (collect keys, sort for deterministic traversal if needed)
  - for each key: filter values, rewrite

Acceptance L4
- Unit tests: delete+set ordering, wildcard delete removes values across all names.

------------------------------------------------------------

L5 — Map-Local with Prefix Stripping + Traversal Protection
5.1 Rule fields:
- type: map_local
- match: methods/host/path regex
- path: string (file or directory; relative resolved against effective_base_path)
- index_file: string (default "index.html")
- status_code: int (default 200)
- content_type: string (optional override)
- response: Ops (delete/set) applied to local response AFTER global response header rules (see pipeline)

5.2 Prefix stripping:
- Use path regex FindStringIndex on URL.Path.
- Must match from index 0:
  - Enforce at config validation time:
    - either require path regex string begins with '^'
    - AND/OR require runtime match start == 0; otherwise treat as no match
- stripped = Path[matchEnd:]
- If stripped == "" => "/"
- If stripped endswith "/" => stripped += index_file

5.3 Mapping:
- Resolve rule.path absolute.
- If file => always serve that file.
- If directory => join directory + stripped (cleaned).
- If not found => 404.

5.4 Traversal protection:
- Normalize stripped:
  - urlPart = path.Clean("/" + stripped) (ensure leading slash)
  - fsRel = strings.TrimPrefix(urlPart, "/")
  - fsRel = filepath.FromSlash(fsRel)
- target = filepath.Join(ruleDir, fsRel)
- absRuleDir, absTarget via filepath.Abs
- ensure absTarget is within absRuleDir (filepath.Rel)
- If traversal => 403.

5.5 Content-Type:
- override if configured
- else mime.TypeByExtension
- else DetectContentType sniff

Acceptance L5
- Unit tests for prefix stripping and traversal safety.

------------------------------------------------------------

L6 — Pipeline (protocol-agnostic core)
6.1 FlowContext:
- Logger
- ClientAddr
- Proto: "http/1.1" | "h2"
- Scheme: http|https
- Host (no port)
- Method
- Path, RawQuery
- Request headers (mutable)
- Request body (io.ReadCloser)

6.2 ResponseResult:
- Status
- Headers
- Body (io.ReadCloser or io.Reader + closer handling)
- Source: "local" | "upstream"

6.3 Pipeline order:
1) Compute metadata
2) Apply REQUEST header rules (matching header rules): delete then set
3) Choose body source:
   - map-local (first match wins) -> local response
   - else upstream roundtrip
4) Apply RESPONSE header rules (matching header rules): delete then set
5) If map-local was used, apply map-local response ops (delete then set) AFTER global response rules
6) Write response
7) Log (slog) with duration_ms, status, source, proto

6.4 Upstream roundtrip:
- Create outbound request to upstream:
  - URL = scheme://host + path + ?query
  - method, headers (after request transforms), body
- Use http.Transport:
  - Proxy=nil
  - TLSClientConfig with InsecureSkipVerify based on insecure_upstream
- Strip hop-by-hop headers.
- NOTE: Upstream HTTP/2 is handled automatically by net/http where available.

Acceptance L6
- Integration tests for rule ordering across local and upstream responses.

------------------------------------------------------------

L7 — CA + Leaf Certs (100-year validity)
7.1 Store in effective data_dir:
- ca.key.pem
- ca.crt.pem
- optional certs/<host>.pem cache

7.2 CA generation:
- RSA 3072 default
- CN "jeltz Root CA"
- validity 100 years
- IsCA=true; KeyUsage CertSign|CRLSign

7.3 Leaf issuance:
- per host
- validity 100 years
- DNSNames include host
- cache in memory, optionally on disk
- thread-safe cache (mutex)

7.4 CLI helpers:
- ca-path prints CA cert path
- ca-install-hint prints install hints

Acceptance L7
- CA created on first run, leaf cert served.

------------------------------------------------------------

L8 — CONNECT MITM with HARD HTTP/2 support inside TLS (ALPN h2)
This layer replaces the CONNECT stub.

8.1 CONNECT flow (outer HTTP/1.1 request):
a) Receive CONNECT host:port
b) Hijack TCP conn (http.Hijacker)
c) Write 200 Connection Established
d) Wrap with tls.Server using tls.Config:
   - GetCertificate from CA (use SNI; fallback to CONNECT host)
   - NextProtos MUST include: ["h2", "http/1.1"]
e) Handshake TLS

8.2 Determine negotiated protocol:
- proto = tlsConn.ConnectionState().NegotiatedProtocol
- If proto == "h2":
  - REQUIRED: serve HTTP/2 on this conn with x/net/http2
- Else:
  - allowed fallback HTTP/1.1 over TLS (log warn that client did not negotiate h2)

8.3 HTTP/2 serving over hijacked TLS conn (REQUIRED)
- Use golang.org/x/net/http2:
  - var h2s http2.Server
  - h2s.ServeConn(tlsConn, &http2.ServeConnOpts{
      Handler: mitmH2Handler(targetHost, targetPort, logger, pipeline),
    })

mitmH2Handler requirements:
- It is an http.Handler closure bound to:
  - target host (from SNI / CONNECT)
  - a shared pipeline instance
- For each request (stream):
  - Construct FlowContext:
    - Proto="h2"
    - Scheme="https"
    - Host=targetHost
    - Method=r.Method
    - Path=r.URL.Path
    - RawQuery=r.URL.RawQuery
    - Headers=r.Header (copy or mutate in-place carefully)
    - Body=r.Body
    - ClientAddr from outer hijack remote addr
  - Run pipeline (L6) -> ResponseResult
  - Write response:
    - apply headers (result headers)
    - w.WriteHeader(status)
    - stream body to w (io.Copy)
    - ensure body closed

Concurrency:
- http2 serves multiple streams concurrently; pipeline + caches MUST be thread-safe.
- Rule sets are immutable post-load (safe).
- Cert cache must be mutex-protected.

8.4 HTTP/1.1 fallback over TLS (optional but recommended)
- If proto != "h2":
  - parse requests with http.ReadRequest in loop
  - run pipeline with Proto="http/1.1"
  - write responses
- This improves compatibility while still meeting the hard h2 requirement.

Acceptance L8
- A client that negotiates ALPN h2 inside CONNECT+TLS can send multiple concurrent requests and get correct responses.

------------------------------------------------------------

L9 — Outer HTTP forwarding uses same pipeline
9.1 For non-CONNECT requests on the outer listener:
- Determine scheme from absolute URL
- Proto="http/1.1"
- Run pipeline
- Respond

Acceptance L9
- HTTP forward requests can be rewritten/mapped.

------------------------------------------------------------

L10 — Operational Hardening
10.1 Timeouts:
- Outer server:
  - ReadHeaderTimeout, IdleTimeout
- Upstream transport:
  - ResponseHeaderTimeout optional

10.2 Cancellation:
- For h2 streams:
  - upstream requests must use r.Context() so cancels propagate per-stream
- For HTTP/1.1:
  - tie context to conn lifetime where possible

10.3 dump_traffic:
- If enabled:
  - log request/response headers
  - log first N bytes of body (N=max_body_bytes), truncating
  - redact Authorization, Cookie by default (future: configurable)

10.4 Errors:
- local file missing => 404
- traversal => 403
- upstream error => 502
- internal => 500
- slog error events with event names:
  - mitm_handshake_error, h2_serve_error, upstream_error, local_file_error, config_error

Acceptance L10
- -race passes for concurrent h2 requests.

============================================================
YAML Configuration Schema (v1)
============================================================

Top-level:
- version: 1
- listen: "127.0.0.1:8080"
- base_path: "."
- data_dir: ""
- insecure_upstream: false
- dump_traffic: false
- max_body_bytes: 1048576
- rules: [ ... ordered ... ]

Rule: header
- type: header
- match:
    methods: [ ... optional ... ]
    host: "regex"
    path: "regex"
- request:
    delete: [ HeaderDeleteOp ... ]
    set:    [ HeaderSetOp ... ]
- response:
    delete: [ HeaderDeleteOp ... ]
    set:    [ HeaderSetOp ... ]

Rule: map_local
- type: map_local
- match:
    methods: [ ... optional ... ]
    host: "regex"
    path: "regex"   # MUST match from start for prefix stripping
- path: "relative-or-absolute"
- index_file: "index.html"
- status_code: 200
- content_type: ""
- response:
    delete: [ ... ]
    set:    [ ... ]

HeaderSetOp:
- name: "X-Debug"
  mode: replace|append
  value: "true"

HeaderDeleteOp (name-based):
- name: "Cookie"
  value_regex: "optional"

HeaderDeleteOp (wildcard):
- any_name: true
  value_regex: "^GDPR=$"

Example config.yaml (v1)
version: 1
listen: "127.0.0.1:8080"
base_path: "."
insecure_upstream: false
rules:
  - type: header
    match:
      methods: ["GET","POST","PUT","DELETE","PATCH","HEAD","OPTIONS","TRACE"]
      host: "^example\\.com$"
      path: "^/api/"
    request:
      delete:
        - any_name: true
          value_regex: "^GDPR=$"
      set:
        - name: "X-Debug"
          mode: replace
          value: "true"
    response:
      set:
        - name: "X-From-Jeltz"
          mode: append
          value: "1"

  - type: map_local
    match:
      methods: ["GET"]
      host: "^example\\.com$"
      path: "^/static/"
    path: "mocks/static"
    index_file: "index.html"
    status_code: 200
    response:
      set:
        - name: "Cache-Control"
          mode: replace
          value: "no-store"

============================================================
HTTP/2 Integration Test Plan (Required)
============================================================

Goal
- Prove that, inside a CONNECT+TLS MITM session, jeltz negotiates ALPN "h2" and correctly serves multiple concurrent HTTP/2 streams.

Test Strategy Overview
- Use a real jeltz instance in-process (spawn server on ephemeral port).
- Use a custom test client that:
  1) opens TCP connection to proxy
  2) performs HTTP/1.1 CONNECT to a dummy target authority (host doesn't need to resolve publicly)
  3) performs TLS handshake to the proxy (as if to the target host) and verifies ALPN negotiated "h2"
  4) speaks HTTP/2 over that TLS connection using x/net/http2
  5) sends multiple concurrent GET requests (separate streams) and validates responses

Core Test Components
A) Start jeltz in test
- Bind listener to 127.0.0.1:0 to get an ephemeral port.
- Use a temp directory for XDG config/data dirs (set env vars in test):
  - XDG_CONFIG_HOME, XDG_DATA_HOME
- Provide a minimal config:
  - one map_local rule mapping /static/ to a temp dir with test files
  - one header rule adding a response header like X-Test: 1
- Ensure CA is generated in temp data dir.

B) Establish CONNECT tunnel
- net.Dial("tcp", proxyAddr)
- write:
  "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"
- read response, assert "200 Connection Established"

C) Perform TLS handshake to proxy-as-target
- Create tls.Config:
  - ServerName: "example.com" (SNI)
  - RootCAs: pool containing jeltz CA cert (load from temp data dir ca.crt.pem)
  - NextProtos: []string{"h2"}   (force offer h2)
- tls.Client(conn, tlsConf).Handshake()
- Assert:
  - tlsConn.ConnectionState().NegotiatedProtocol == "h2"

D) Speak HTTP/2 on the TLS conn
- Use golang.org/x/net/http2:
  - clientConn, err := http2.NewClientConn(tlsConn)
    (Note: exact API varies by version; use the correct current x/net/http2 client entry points.)
- Construct an *http.Request with:
  - Method GET
  - URL: "https://example.com/static/file1.txt"
  - Host: example.com
  - (Path/query important; authority must match target host)
- Send N concurrent requests (e.g., 20) using goroutines:
  - Each reads full body and checks it matches local file content
  - Check response header X-Test exists and equals "1"
  - Confirm status 200

E) Also test upstream pass-through path (optional but recommended)
- Start a local upstream HTTPS server (httptest.NewTLSServer) with known content.
- Configure jeltz to proxy to that host for non-map-local paths.
- Because MITM uses the target host from SNI, use:
  - target host set to the httptest server host
  - NOTE: this requires CONNECT to that host and SNI to that host.
- Verify response header rewriting applies.

F) Verify logging includes proto="h2"
- If tests can capture logs (slog handler to buffer), assert at least one event includes proto=h2.

Key Assertions
- CONNECT succeeds.
- TLS handshake negotiates ALPN "h2".
- At least 2 concurrent streams succeed without blocking each other.
- Map-local returns correct content and applies header rule(s).
- No data races (`go test -race`).

Notes / Version Compatibility
- x/net/http2 client APIs can differ; pin a module version in go.mod.
- If client API friction is too high, alternate approach:
  - Use an http.Client with custom Transport that uses an already-established TLS conn (more complex).
  - Prefer direct http2 client conn if available.

============================================================
Definition of Done
============================================================
- YAML config managed by viper; strict validation with yaml.v3.
- XDG-compliant paths, portable config via base_path.
- Structured logging with slog.
- HTTPS MITM:
  - CA generation + 100-year leaf cert issuance
  - ALPN includes "h2"
  - HTTP/2 server on hijacked TLS conn via x/net/http2
- Rules:
  - structured host/path/method regex
  - map-local prefix stripping + traversal protection
  - header ops: append/replace/delete + wildcard delete by value regex
  - response rules apply to upstream and local responses
- Automated tests:
  - unit tests for rules/ops
  - integration test verifying h2 negotiation and concurrent streams
  - `go test -race` clean

