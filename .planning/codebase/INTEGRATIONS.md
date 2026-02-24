# External Integrations

**Analysis Date:** 2026-02-24

## APIs & External Services

**External HTTP(S) upstreams:**
- Arbitrary target servers requested by proxy clients
  - Client/Transport: `net/http.Transport` configured in `internal/proxy/pipeline.go`
  - Auth: Pass-through request headers; no built-in upstream credential store

**No third-party SaaS SDK integrations detected:**
- No Stripe/Supabase/AWS/GCP vendor SDK imports in `cmd/`, `internal/`, or `pkg/`

## Data Storage

**Databases:**
- Not detected

**File Storage:**
- Local filesystem only
  - CA assets stored in data dir from XDG resolution (`pkg/xdg/xdg.go`)
  - Files: `ca.key.pem`, `ca.crt.pem`, `ca.p12` managed in `internal/ca/ca.go`
  - `map_local` serves files/directories from local paths in `internal/rules/maplocal.go`

**Caching:**
- In-memory leaf cert LRU cache (`leafCacheMaxEntries = 1024`) in `internal/ca/ca.go`

## Authentication & Identity

**Auth Provider:**
- None (no user/session auth system)

**TLS trust model:**
- Local root CA generated and loaded by `internal/ca/ca.go`
- Per-host leaf certs issued by `pkg/ca/ca.go`

## Monitoring & Observability

**Error Tracking:**
- None detected (no Sentry/Honeycomb/Datadog SDK)

**Logs:**
- Structured `slog` logs to stderr configured in `internal/logging/logging.go`

## CI/CD & Deployment

**Hosting:**
- Not detected (binary tool, local run model)

**CI Pipeline:**
- GitHub Actions in `.github/workflows/test.yml` running `go test ./...` on Linux/macOS/Windows

## Environment Configuration

**Required env vars:**
- Optional runtime overrides: `JELTZ_LISTEN`, `JELTZ_BASE_PATH`, `JELTZ_DATA_DIR`, `JELTZ_INSECURE_UPSTREAM`, `JELTZ_DUMP_TRAFFIC`, `JELTZ_MAX_BODY_BYTES`, `JELTZ_MAX_UPSTREAM_REQUEST_BODY_BYTES`, `JELTZ_UPSTREAM_DIAL_TIMEOUT_MS`, `JELTZ_UPSTREAM_TLS_HANDSHAKE_TIMEOUT_MS`, `JELTZ_UPSTREAM_RESPONSE_HEADER_TIMEOUT_MS`, `JELTZ_UPSTREAM_IDLE_CONN_TIMEOUT_MS` (parsed in `internal/config/config.go`)
- Optional XDG base vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME` resolved in `pkg/xdg/xdg.go`

**Secrets location:**
- Local CA private key file `ca.key.pem` under data dir managed by `internal/ca/ca.go`

## Webhooks & Callbacks

**Incoming:**
- None (proxy listener only, no webhook endpoints)

**Outgoing:**
- Outbound upstream HTTP/HTTPS requests performed by `internal/proxy/pipeline.go`

---

*Integration audit: 2026-02-24*
