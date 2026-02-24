# jeltz

## What This Is

A single-binary TLS-intercepting HTTP/HTTPS forward proxy (MITM proxy) for local development. It issues per-host leaf certificates on demand, applies configurable request/response header rules, and can serve local files in place of upstream responses. Aimed at developers who need to inspect or modify browser traffic without installing a heavyweight tool.

## Core Value

Intercept and modify HTTPS traffic transparently — any rule change takes effect without touching the browser or OS trust store again.

## Requirements

### Validated

- ✓ MITM interception of HTTPS via dynamic per-host leaf certificate issuance — existing
- ✓ Plain HTTP forward proxy support — existing
- ✓ YAML config file with `JELTZ_*` env var and CLI flag override layers — existing
- ✓ Request and response header manipulation rules (add/delete) — existing
- ✓ `map_local` rule: serve local files instead of upstream responses — existing
- ✓ Traffic dump (`-dump-traffic` flag) for request/response inspection — existing
- ✓ HTTP/2 support on the MITM tunnel leg via `x/net/http2` — existing
- ✓ CA management subcommands (`ca-path`, `ca-p12-path`, `ca-install-hint`) — existing
- ✓ PKCS#12 bundle export for browser/OS CA import — existing
- ✓ Graceful shutdown on SIGINT/SIGTERM — existing
- ✓ XDG Base Directory support for config and data paths — existing

### Active

- [ ] Config file is read exactly once on startup; the single in-memory byte slice is shared across Viper initialisation, strict `KnownFields` validation, and rule struct parsing

### Out of Scope

- Upstream transport timeouts — not requested for this cycle
- `map_local` streaming via `http.ServeContent` — not requested for this cycle
- `io.TeeReader` fix for `-dump-traffic` truncation — not requested for this cycle
- Per-host cert cache eviction — not requested for this cycle
- Windows build target — not requested for this cycle

## Context

Brownfield Go project. Codebase mapped 2026-02-24. Config loading is the only active improvement target. The fix is confined to `internal/config/config.go`: read the YAML file once into `[]byte`, feed that slice to Viper (via `viper.SetConfigType` + `viper.ReadConfig(bytes.NewReader(raw))`), to `yaml.Decoder` with `KnownFields(true)` for strict validation, and to `yaml.Unmarshal` for rule struct parsing.

## Constraints

- **Tech stack**: Go only — no new direct dependencies
- **Scope**: Changes confined to `internal/config/config.go`; public API (`config.Load` signature) unchanged

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Fix config triple-read before other concerns | Smallest change, zero risk of regression, cleanest starting point | — Pending |
| Keep `config.Load` signature unchanged | Callers (`cmd/jeltz/main.go`) must not need updating | — Pending |

---
*Last updated: 2026-02-24 after initialization*
