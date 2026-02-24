# jeltz

A local developer HTTPS proxy with MITM interception, rule-based request/response rewriting, and local file serving.

Named after Prostetnic Vogon Jeltz from *The Hitchhiker's Guide to the Galaxy*.

---

## What it does

Point your browser (or any HTTP client) at jeltz as an explicit HTTP proxy. For plain HTTP traffic it forwards requests and applies rules. For HTTPS traffic it performs MITM interception: it terminates the TLS connection from the client using a locally-generated certificate, applies your rules, then proxies the request to the real upstream.

HTTP/2 is fully supported on the client-to-proxy leg of HTTPS connections. The client-facing listener is HTTP/1.1 (standard for explicit proxies).

**Use cases:**
- Serve local mock files instead of real upstream responses (`map_local`)
- Inject, replace, or strip request and response headers
- Strip tracking cookies, GDPR consent headers, etc.
- Inspect traffic with structured logging

---

## Requirements

- Go 1.26+

---

## Build

```sh
git clone https://github.com/fabiant7t/jeltz
cd jeltz
make build        # produces ./jeltz binary
make test         # run tests
make race         # run tests with race detector
```

---

## Quick start

```sh
# 1. Install the CA certificate so your browser trusts jeltz's MITM certs
jeltz ca-install-hint

# 2. Write a config (optional — jeltz works with no config too)
mkdir -p ~/.config/jeltz
cat > ~/.config/jeltz/config.yaml <<'EOF'
version: 1
listen: "127.0.0.1:8080"
rules: []
EOF

# 3. Start the proxy
jeltz

# 4. Point your browser or tool at http://127.0.0.1:8080
#    e.g. curl --proxy http://127.0.0.1:8080 https://example.com/
```

On first run jeltz generates a root CA (`ca.key.pem` + `ca.crt.pem`) in its data directory. All HTTPS leaf certificates are issued from this CA and cached on disk.

---

## CA setup

### Print the CA certificate path

```sh
jeltz ca-path
```

### Print the CA bundle path

```sh
jeltz ca-p12-path
```

### Print platform-specific installation instructions

```sh
jeltz ca-install-hint
```

On first run jeltz writes three files to its data directory:

| File | Use |
|---|---|
| `ca.crt.pem` | PEM certificate — use with `security`, `certutil`, `update-ca-certificates` |
| `ca.key.pem` | Private key — keep private |
| `ca.p12` | PKCS#12 bundle (password: `jeltz`) — use on Windows or Firefox import |

The CA certificate must be trusted by your OS or browser before HTTPS interception works without certificate errors.

---

## CLI flags

| Flag | Default | Description |
|---|---|---|
| `-listen` | `127.0.0.1:8080` | Proxy listen address |
| `-config` | `~/.config/jeltz/config.yaml` (if it exists) | Path to config file |
| `-base-path` | XDG config dir | Base path for resolving relative `path` values in rules |
| `-data-dir` | XDG data dir (`~/.local/share/jeltz`) | Directory for CA key/cert and leaf cert cache |
| `-log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `-insecure-upstream` | `false` | Skip TLS certificate verification for upstream connections |
| `-dump-traffic` | `false` | Log request/response headers and body snippets at debug level |
| `-max-body-bytes` | `1048576` | Max body bytes to log when `-dump-traffic` is enabled |
| `-upstream-dial-timeout-ms` | `10000` | Upstream TCP dial timeout in milliseconds |
| `-upstream-tls-handshake-timeout-ms` | `10000` | Upstream TLS handshake timeout in milliseconds |
| `-upstream-response-header-timeout-ms` | `30000` | Upstream response header timeout in milliseconds |
| `-upstream-idle-conn-timeout-ms` | `60000` | Upstream idle connection timeout in milliseconds |

CLI flags override config file values. For bool/int flags, overrides apply only when the flag is explicitly provided on the CLI. Config file values override environment variables (`JELTZ_` prefix). Environment variables override built-in defaults.

---

## Configuration

The config file is YAML. Unknown keys are rejected. The only supported version is `1`.

```yaml
version: 1
listen: "127.0.0.1:8080"    # proxy listen address
base_path: "."               # base for relative rule paths; "." = config dir
data_dir: ""                 # CA/cert storage; empty = XDG data dir
insecure_upstream: false     # skip TLS verification upstream
dump_traffic: false          # log headers + body snippets
max_body_bytes: 1048576      # body dump limit in bytes
upstream_dial_timeout_ms: 10000              # upstream TCP dial timeout
upstream_tls_handshake_timeout_ms: 10000     # upstream TLS handshake timeout
upstream_response_header_timeout_ms: 30000   # upstream response header timeout
upstream_idle_conn_timeout_ms: 60000         # upstream idle conn timeout
rules: []                    # ordered list of rules
```

### Path resolution

- `base_path` empty or `"."` → the directory containing `config.yaml` (XDG config dir)
- `base_path` relative → resolved relative to the XDG config dir
- `base_path` absolute → used as-is (less portable)
- Rule `path` values that are relative are resolved against `base_path`

---

## Rules

Rules are evaluated in file order. All matching header rules apply to every request/response. For `map_local`, the first matching rule wins.

### Pipeline order (per request)

1. Apply matching **request** header rules (delete then set)
2. Check `map_local` rules (first match wins) — or proxy to upstream
3. Apply matching **response** header rules (delete then set)
4. Apply `map_local` rule's own `response` ops (after global response rules)

---

### Rule: `header`

Transforms request and/or response headers for matching traffic.

```yaml
- type: header
  match:
    methods: ["GET", "POST"]  # optional; omit to match any method
    host: "^example\\.com$"   # required; regex matched against hostname only (no port)
    path: "^/api/"            # required; regex matched against URL path
  request:
    delete:
      - name: "Cookie"              # delete all values of this header
      - name: "Cookie"              # delete only values matching the pattern
        value: "^session="
      - any_name: true              # delete values matching regex across ALL headers
        value: "^GDPR=$"
    set:
      - name: "X-Debug"
        mode: replace               # replace: overwrite any existing values
        value: "true"
      - name: "X-Request-Id"
        mode: append                # append: add alongside existing values
        value: "jeltz"
  response:
    set:
      - name: "X-From-Jeltz"
        mode: append
        value: "1"
```

**Delete op variants:**

| Fields | Behaviour |
|---|---|
| `name: "Foo"` | Remove all values of header `Foo` |
| `name: "Foo"`, `value: "^bar"` | Remove only values of `Foo` matching the regex pattern; keep the rest |
| `any_name: true`, `value: "^bar"` | Remove any value matching the regex pattern across every header name |

`value` in a delete op is a **regex pattern**. `value` in a set op is a **literal string**.

Delete ops run before set ops within the same block.

---

### Rule: `map_local`

Serve a local file or directory instead of proxying to the upstream.

```yaml
- type: map_local
  match:
    methods: ["GET"]
    host: "^example\\.com$"
    path: "^/static/"          # MUST start with ^ (required for prefix stripping)
  path: "mocks/static"         # relative to base_path, or absolute
  index_file: "index.html"     # served when the stripped path ends with /  (default: index.html)
  status_code: 200             # response status (default: 200)
  content_type: ""             # override Content-Type; empty = auto-detect
  response:                    # extra response header ops applied after global rules
    set:
      - name: "Cache-Control"
        mode: replace
        value: "no-store"
```

**Prefix stripping:** the matched prefix (the part of the URL path the `path` regex consumed) is stripped before looking up the file. `/static/js/app.js` with regex `^/static/` resolves to `mocks/static/js/app.js`.

The `path` regex must begin with `^`. Traversal attempts (`../../`) are neutralised by URL path normalisation and a filesystem containment check.

If `path` points to a file, that file is always served regardless of the URL path. If it points to a directory, the stripped URL path is joined to it.

Content-Type is determined by: explicit `content_type` → file extension → `Content-Type` sniffing → `application/octet-stream`.

---

## Full example config

```yaml
version: 1
listen: "127.0.0.1:8080"
base_path: "."
insecure_upstream: false
upstream_dial_timeout_ms: 10000
upstream_tls_handshake_timeout_ms: 10000
upstream_response_header_timeout_ms: 30000
upstream_idle_conn_timeout_ms: 60000

rules:
  # Strip GDPR consent cookies from every request
  - type: header
    match:
      host: ".*"
      path: ".*"
    request:
      delete:
        - any_name: true
          value: "^gdpr_consent="

  # Add a debug header and strip X-Powered-By from responses to the API
  - type: header
    match:
      methods: ["GET", "POST", "PUT", "PATCH", "DELETE"]
      host: "^api\\.example\\.com$"
      path: "^/"
    request:
      set:
        - name: "X-Debug"
          mode: replace
          value: "1"
    response:
      delete:
        - name: "X-Powered-By"

  # Serve local mock files for the frontend's static assets
  - type: map_local
    match:
      methods: ["GET"]
      host: "^www\\.example\\.com$"
      path: "^/static/"
    path: "mocks/static"
    response:
      set:
        - name: "Cache-Control"
          mode: replace
          value: "no-store"

  # Always return a fixed JSON fixture for one endpoint
  - type: map_local
    match:
      methods: ["GET"]
      host: "^api\\.example\\.com$"
      path: "^/v1/features"
    path: "mocks/features.json"
    content_type: "application/json"
```

---

## Logging

jeltz writes structured text logs to stderr. All events share a stable set of keys:

| Key | Description |
|---|---|
| `component` | Subsystem (`main`, `proxy`, `mitm`, `pipeline`, `dump`) |
| `event` | Named error events (`config_error`, `mitm_handshake_error`, `h2_serve_error`, `upstream_error`, `local_file_error`) |
| `client` | Client IP:port |
| `method` | HTTP method |
| `scheme` | `http` or `https` |
| `host` | Target hostname |
| `path` | URL path |
| `status` | Response status code |
| `source` | `local` (map_local) or `upstream` |
| `duration_ms` | Request duration in milliseconds |
| `proto` | `http/1.1` or `h2` |
| `error` | Error message |

### Traffic dumping

Start with `-dump-traffic` to log request/response headers at `debug` level after all transforms are applied. `Authorization`, `Cookie`, and `Set-Cookie` headers are redacted. Body snippets (up to `-max-body-bytes`) are also logged.

```sh
jeltz -log-level debug -dump-traffic
```

---

## Files and directories

| Path | Contents |
|---|---|
| `~/.config/jeltz/config.yaml` | Default config file location |
| `~/.local/share/jeltz/ca.crt.pem` | Root CA certificate (trust this) |
| `~/.local/share/jeltz/ca.key.pem` | Root CA private key (keep private) |
| `~/.local/share/jeltz/ca.p12` | Root CA PKCS#12 bundle, password `jeltz` (trust this) |
| `~/.local/share/jeltz/certs/<host>.pem` | Cached leaf certificates |

Locations follow the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/). Override with `$XDG_CONFIG_HOME` and `$XDG_DATA_HOME`.

---

## Scope and non-goals (v1)

- The outer listener (the port you configure as your HTTP proxy) is **HTTP/1.1 only**. HTTP/2 is supported only on the decrypted client-to-jeltz leg inside CONNECT tunnels.
- No WebSocket support.
- No transparent/intercepting proxy (requires iptables/TPROXY).
- No web UI or TUI.
- `map_remote` rule type is not implemented (the config model anticipates it).
