# Technology Stack

**Analysis Date:** 2026-02-24

## Languages

**Primary:**
- Go 1.25 module target (`go.mod`) - Core implementation in `cmd/`, `internal/`, and `pkg/`

**Secondary:**
- YAML config schema (`config.yaml`) parsed by `gopkg.in/yaml.v3` in `internal/config/config.go`

## Runtime

**Environment:**
- Native Go binary runtime (`jeltz`) started from `cmd/jeltz/main.go`

**Package Manager:**
- Go modules (`go.mod` / `go.sum`)
- Lockfile: present (`go.sum`)

## Frameworks

**Core:**
- Go standard library (`net/http`, `crypto/tls`, `log/slog`, `os`, `context`) - proxy server, TLS MITM, logging, file I/O

**Networking extensions:**
- `golang.org/x/net/http2` - HTTP/2 handling on client-to-proxy TLS leg in `internal/proxy/mitm.go`

**Configuration:**
- `gopkg.in/yaml.v3` - strict KnownFields YAML decode in `internal/config/config.go`

**Testing:**
- Go stdlib `testing` + `net/http/httptest` across `*_test.go` files

**Build/Release:**
- `make` targets in `Makefile`
- GoReleaser config in `.goreleaser.yaml`

## Key Dependencies

**Critical:**
- `golang.org/x/net v0.50.0` (`go.mod`) - provides HTTP/2 server/client transport integration used by MITM path
- `gopkg.in/yaml.v3 v3.0.1` (`go.mod`) - config decode and schema validation

**Infrastructure:**
- `log/slog` (stdlib) - structured logs via `internal/logging/logging.go`
- `crypto/x509`, `crypto/rsa`, `crypto/tls` (stdlib) - CA generation and leaf issuance in `pkg/ca/ca.go` and `internal/ca/ca.go`

## Configuration

**Environment:**
- Runtime overrides via `JELTZ_*` env vars parsed in `internal/config/config.go`
- Config file path from `-config` flag (`cmd/jeltz/main.go`) with strict YAML schema (`version: 1`)

**Build:**
- Build metadata injected with `-ldflags` in `Makefile` and `.goreleaser.yaml`
- CI matrix in `.github/workflows/test.yml`

## Platform Requirements

**Development:**
- Go toolchain compatible with `go.mod` (`go 1.25.0`)
- `make` for local shortcuts (`make build`, `make test`, `make race`)

**Production/Runtime target:**
- Single static binary (`CGO_ENABLED=0`) targeting Linux and macOS in `.goreleaser.yaml`

---

*Stack analysis: 2026-02-24*
