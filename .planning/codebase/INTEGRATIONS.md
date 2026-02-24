# External Integrations

**Analysis Date:** 2026-02-24

## APIs & Services

**None.** jeltz is a local developer tool. It does not call any external APIs or third-party services on its own behalf. All network traffic it handles is user-initiated proxy traffic forwarded to destinations the user's clients request.

## Databases / Storage

**Databases:**
- None. No database dependency.

**File Storage (local filesystem only):**

CA and certificate storage at `~/.local/share/jeltz/` (XDG data dir; overridable via `$XDG_DATA_HOME` or `-data-dir` flag):
- `ca.key.pem` — RSA root CA private key
- `ca.crt.pem` — Root CA certificate (PEM)
- `ca.p12` — PKCS#12 bundle for Windows/Firefox import (password: `jeltz`)
- `certs/<host>.pem` — Per-host leaf certificate cache (PEM, cert + key)

Configuration file at `~/.config/jeltz/config.yaml` (XDG config dir; overridable via `$XDG_CONFIG_HOME` or `-config` flag).

Rule `map_local` mock files: user-defined paths on local disk, resolved against `base_path` (default: XDG config dir).

**Caching:**
- In-memory per-host TLS leaf certificate cache in `internal/ca/ca.go` (`CA.cache` map protected by `sync.Mutex`)
- On-disk leaf cert cache at `~/.local/share/jeltz/certs/<host>.pem` — loaded on startup, written on first issuance per host

## Infrastructure

**Hosting:**
- Runs as a local process on the developer's machine; no cloud deployment
- Release binaries distributed as `tar.gz` archives built by `goreleaser` (`.goreleaser.yaml`)
- Target platforms: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`

**CI Pipeline:**
- Not detected — no `.github/`, `.gitlab-ci.yml`, or equivalent CI config present

**Container/IaC:**
- None detected

## Third-party Libraries (notable)

**`golang.org/x/net/http2` (v0.50.0):**
- Used in `internal/proxy/mitm.go` (`serveH2`) to serve HTTP/2 connections over hijacked TLS connections inside CONNECT tunnels
- Provides `http2.Server.ServeConn` for per-connection H2 handling
- Without this package, HTTP/2 MITM interception is unavailable

**`github.com/spf13/viper` (v1.21.0):**
- Used exclusively in `internal/config/config.go` for layered config loading: YAML file → env vars (`JELTZ_` prefix) → defaults
- Brings 9 indirect dependencies (see `STACK.md`)

**`gopkg.in/yaml.v3` (v3.0.1):**
- Used in `internal/config/config.go` for strict YAML decoding (`KnownFields(true)` rejects unknown config keys)
- Also used for rule struct parsing (viper loses type info for nested structures)

**Custom PKCS#12 encoder (`pkg/p12/p12.go`):**
- Stdlib-only PFX v3 encoder (RFC 7292) — no third-party PKCS#12 library
- Format: unencrypted CertBag + PBE-SHA1-3DES ShroudedKeyBag + HMAC-SHA1 MAC
- Used to produce `ca.p12` for browser and Windows CA trust import

**TLS / PKI (stdlib `crypto/*`):**
- `crypto/tls`, `crypto/x509`, `crypto/rsa`, `crypto/rand` — all stdlib
- CA generation and leaf cert issuance implemented in `pkg/ca/ca.go`
- CA loading, caching, and on-disk persistence in `internal/ca/ca.go`

## Environment Variables

| Variable | Purpose |
|---|---|
| `JELTZ_LISTEN` | Proxy listen address |
| `JELTZ_BASE_PATH` | Base path for rule file resolution |
| `JELTZ_DATA_DIR` | CA and certificate storage directory |
| `JELTZ_INSECURE_UPSTREAM` | Skip upstream TLS verification |
| `JELTZ_DUMP_TRAFFIC` | Enable traffic header/body dumping |
| `JELTZ_MAX_BODY_BYTES` | Body dump byte limit |
| `XDG_CONFIG_HOME` | Override XDG config base directory |
| `XDG_DATA_HOME` | Override XDG data base directory |

## Webhooks & Callbacks

**Incoming:** None — jeltz receives standard HTTP/HTTPS forward proxy requests, not webhooks.

**Outgoing:** None — jeltz does not emit webhooks or call external notification endpoints.

---

*Integration audit: 2026-02-24*
