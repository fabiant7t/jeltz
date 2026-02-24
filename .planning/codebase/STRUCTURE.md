# Codebase Structure

**Analysis Date:** 2026-02-24

## Directory Layout

```text
jeltz/
├── cmd/jeltz/              # CLI entrypoint, flags, subcommands, banner
├── internal/ca/            # CA lifecycle and leaf cert caching
├── internal/config/        # YAML/env/CLI config resolution
├── internal/proxy/         # Server, MITM, request pipeline
├── internal/rules/         # Rule compilation and evaluation
├── internal/httpx/         # HTTP header utility helpers
├── internal/logging/       # slog setup and stable key names
├── pkg/ca/                 # Pure certificate generation primitives
├── pkg/p12/                # PKCS#12 encoding implementation
├── pkg/xdg/                # XDG path resolution helpers
├── .github/workflows/      # CI workflows
├── .planning/codebase/     # Maintained analysis documents
├── go.mod                  # Module dependencies
├── Makefile                # Build/test/release commands
└── README.md               # User and operator documentation
```

## Directory Purposes

**`cmd/jeltz/`:**
- Purpose: executable behavior and CLI surface
- Contains: `main.go`, banner rendering (`banner.go`), CLI tests (`*_test.go`)
- Key files: `cmd/jeltz/main.go`, `cmd/jeltz/cli_output_test.go`

**`internal/proxy/`:**
- Purpose: HTTP proxy runtime, MITM handling, and flow pipeline
- Contains: request handlers (`proxy.go`), MITM ALPN paths (`mitm.go`), flow pipeline (`pipeline.go`)
- Key files: `internal/proxy/proxy.go`, `internal/proxy/pipeline.go`, `internal/proxy/mitm.go`

**`internal/rules/`:**
- Purpose: compile YAML rules and evaluate request/response/header/map_local behavior
- Contains: rule compilation and operation application
- Key files: `internal/rules/ruleset.go`, `internal/rules/headers.go`, `internal/rules/maplocal.go`

**`internal/config/`:**
- Purpose: config source precedence and strict schema parsing
- Contains: `Config` schema, env parsing, path resolution
- Key files: `internal/config/config.go`

**`internal/ca/` + `pkg/ca/` + `pkg/p12/`:**
- Purpose: CA persistence/cache and low-level crypto primitives
- Contains: disk I/O and caches in `internal/ca`; pure cert issuance in `pkg/ca`; P12 encoder in `pkg/p12`
- Key files: `internal/ca/ca.go`, `pkg/ca/ca.go`, `pkg/p12/p12.go`

## Key File Locations

**Entry Points:**
- `cmd/jeltz/main.go`: process startup and subcommand routing

**Configuration:**
- `internal/config/config.go`: defaults/env/file/CLI merge and validation
- `README.md`: user-facing config reference and rule examples

**Core Logic:**
- `internal/proxy/pipeline.go`: request/response flow execution
- `internal/proxy/mitm.go`: CONNECT TLS interception and HTTP/2 support
- `internal/rules/ruleset.go`: compile-time rule type dispatch

**Testing:**
- `internal/proxy/*_test.go`: unit + integration proxy tests
- `internal/config/config_test.go`: precedence and validation tests
- `internal/rules/*_test.go`: matching/header/map_local tests
- `pkg/*/*_test.go`: package-level primitive tests

## Naming Conventions

**Files:**
- Lowercase short names by domain (`pipeline.go`, `maplocal.go`, `xdg.go`)
- Tests use `_test.go` suffix beside package files

**Directories:**
- Domain-oriented package layout (`internal/proxy`, `internal/rules`, `pkg/ca`)

## Where to Add New Code

**New proxy feature:**
- Primary code: `internal/proxy/`
- Rules/compiler changes: `internal/rules/`
- Config wiring: `internal/config/config.go` + `cmd/jeltz/main.go`
- Tests: corresponding `internal/proxy/*_test.go` and/or `internal/rules/*_test.go`

**New CLI behavior:**
- Implementation: `cmd/jeltz/main.go` (or split helper file in `cmd/jeltz/`)
- Output/behavior tests: `cmd/jeltz/main_test.go`, `cmd/jeltz/cli_output_test.go`

**New utility helper:**
- Internal-only helper: add under `internal/<domain>/`
- Reusable pure helper: add under `pkg/<domain>/`

## Special Directories

**`.planning/codebase/`:**
- Purpose: operational reference docs for planning/execution commands
- Generated: No (manually curated)
- Committed: Yes

**`.github/workflows/`:**
- Purpose: CI execution definition
- Generated: No
- Committed: Yes

---

*Structure analysis: 2026-02-24*
