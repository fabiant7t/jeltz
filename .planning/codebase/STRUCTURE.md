# Codebase Structure

**Analysis Date:** 2026-02-24

## Top-level Layout

```
jeltz/                         # Module root: github.com/fabiant7t/jeltz
├── cmd/
│   └── jeltz/                 # Main binary entry point
│       ├── main.go            # CLI parsing, startup orchestration, subcommands
│       └── banner.go          # Startup banner rendering (ANSI color, version info)
├── internal/                  # Application-internal packages (not importable externally)
│   ├── ca/                    # Root CA lifecycle: load/generate/cache leaf certs
│   ├── config/                # Config loading: YAML + Viper + env + CLI overrides
│   ├── httpx/                 # Shared HTTP utilities (hop-by-hop header stripping)
│   ├── logging/               # slog setup and stable key constants
│   ├── proxy/                 # HTTP/HTTPS proxy server, MITM handler, pipeline
│   └── rules/                 # Rule engine: match, header ops, map-local serving
├── pkg/                       # Pure, reusable library packages (no internal imports)
│   ├── ca/                    # Crypto primitives: GenerateCA, IssueLeaf
│   ├── p12/                   # Pure-Go PKCS#12 (PFX) encoder
│   └── xdg/                   # XDG Base Directory resolution
├── go.mod                     # Module declaration, Go 1.25+, direct dependencies
├── go.sum                     # Dependency checksums
├── Makefile                   # build, test, race, lint, clean, release targets
├── .goreleaser.yaml           # Release build config (linux/darwin, amd64/arm64)
├── README.md                  # User-facing documentation
├── LICENSE                    # License file
└── .gitignore
```

## Key Files

**Entry point:**
- `cmd/jeltz/main.go` — `func main()`: parses CLI flags, resolves XDG dirs, loads config, loads CA, compiles rules, starts the proxy server. Also contains `runCAPath()`, `runCAP12Path()`, `runCAInstallHint()` subcommand functions.
- `cmd/jeltz/banner.go` — `printBanner()`: renders the startup summary to stderr with optional ANSI color. Reads build-time variables (`version`, `buildDate`, `gitRevision`) injected via `-ldflags`.

**Proxy core:**
- `internal/proxy/proxy.go` — `Server` struct, `ListenAndServe`, `ServeHTTP`, `handleCONNECT`, `handleForward`, `rawTunnel`. Defines the `caLoader` interface.
- `internal/proxy/mitm.go` — `mitmHandler`, `serveH2`, `serveHTTP1`, `writeHTTP1Response`. TLS interception and per-protocol request serving.
- `internal/proxy/pipeline.go` — `FlowContext`, `ResponseResult`, `Pipeline`, `Pipeline.Run`. The complete request→response processing chain including rule application, local file serving, and upstream round-trip.

**Rule engine:**
- `internal/rules/rules.go` — `FlowMeta`, `Match`, `CompileMatch`. Core matching types.
- `internal/rules/ruleset.go` — `RuleSet`, `Compile`. Dispatches raw config rules to typed compiled forms.
- `internal/rules/headers.go` — `Ops`, `DeleteOp`, `SetOp`, `CompileOps`, `Apply`. Header mutation operations.
- `internal/rules/maplocal.go` — `MapLocalRule`, `MapLocalResult`, `CompileMapLocalRule`, `Resolve`, `DetectContentType`. Filesystem serving with traversal protection.

**Configuration:**
- `internal/config/config.go` — `Config`, `RawRule`, `RawMatch`, `RawOps`, `CLIOverrides`, `Load`. Defines all YAML-mapped types and the full config resolution pipeline.

**Certificate authority:**
- `internal/ca/ca.go` — `CA` struct, `Load`, `LeafCert`. Manages CA key/cert on disk, in-memory leaf cert cache, and PKCS#12 bundle. Depends on `pkg/ca` and `pkg/p12`.
- `pkg/ca/ca.go` — `GenerateCA`, `IssueLeaf`. Pure crypto, no disk I/O.
- `pkg/p12/p12.go` — `Encode`. Pure-Go PKCS#12 DER encoding, no third-party deps.

**Utilities:**
- `internal/logging/logging.go` — `New(level)`, slog key constants.
- `internal/httpx/hopbyhop.go` — `RemoveHopByHop(h http.Header)`.
- `pkg/xdg/xdg.go` — `ConfigDir(appName)`, `DataDir(appName)`.

**Tests (co-located with source):**
- `internal/ca/ca_test.go`
- `internal/config/config_test.go`
- `internal/proxy/pipeline_test.go`
- `internal/proxy/mitm_h2_integration_test.go`
- `internal/rules/headers_test.go`
- `internal/rules/maplocal_test.go`
- `internal/rules/rules_test.go`
- `pkg/ca/ca_test.go`
- `pkg/p12/p12_test.go`
- `pkg/xdg/xdg_test.go`

## Module Organization

**Module path:** `github.com/fabiant7t/jeltz`

**Package split principle:**
- `internal/` — packages that depend on application concerns (config types, slog logger, other internal packages). Cannot be imported by external modules.
- `pkg/` — packages that are self-contained, pure, and have no imports from `internal/`. Suitable for use as a library. Currently contains: crypto primitives (`pkg/ca`), PKCS#12 encoding (`pkg/p12`), XDG paths (`pkg/xdg`).

**Dependency direction (no cycles):**
```
cmd/jeltz
  → internal/ca        → pkg/ca, pkg/p12
  → internal/config    (stdlib + viper + yaml.v3)
  → internal/logging   (stdlib)
  → internal/proxy     → internal/ca, internal/httpx, internal/logging, internal/rules
  → internal/rules     → internal/config
  → pkg/xdg            (stdlib)
```

`pkg/` packages depend only on the standard library.
`internal/config` is a leaf package (no internal imports).
`internal/logging` is a leaf package.
`internal/httpx` is a leaf package.

## Naming Conventions

**Files:** `lowercase.go`, single word or two-word concatenation (e.g., `hopbyhop.go`, `maplocal.go`). Test files: `*_test.go`.

**Packages:** Lowercase, one word, matching the directory name (e.g., `package proxy`, `package rules`, `package ca`).

**Types:** PascalCase structs (`FlowContext`, `ResponseResult`, `MapLocalRule`). Interfaces named by capability (`caLoader`).

**Functions:** PascalCase for exported (`CompileMatch`, `RemoveHopByHop`), camelCase for unexported (`applyDelete`, `serveLocal`).

## Where to Add New Code

**New rule type:**
- Add raw config struct fields to `internal/config/config.go` (`RawRule` struct)
- Implement compiled type and `Compile*` function in a new file under `internal/rules/` (follow pattern of `maplocal.go`)
- Add a new slice field to `RuleSet` in `internal/rules/ruleset.go` and a case in `Compile()`
- Apply the rule in the appropriate step inside `Pipeline.Run` in `internal/proxy/pipeline.go`

**New CLI subcommand:**
- Add a `case` in the `switch os.Args[1]` block in `cmd/jeltz/main.go`
- Implement as a `run*()` function in `cmd/jeltz/main.go` (or a new file in `cmd/jeltz/`)

**New reusable crypto/utility primitive:**
- Add a new package under `pkg/` with no imports from `internal/`

**New internal utility:**
- Add to an appropriate existing package under `internal/` or create a new one-file package

**Configuration options:**
- Add field to `yamlConfig` struct (YAML schema) and `Config` struct in `internal/config/config.go`
- Wire through `config.Load()` and add a CLI flag + `CLIOverrides` field in `cmd/jeltz/main.go`

## Special Directories

**`.planning/`:**
- Purpose: GSD planning documents (architecture, conventions, concerns, etc.)
- Generated: No (written by planning agents)
- Committed: Yes (in `.gitignore` it is NOT excluded — tracked)

**`cmd/jeltz/`:**
- Purpose: The single binary produced by this module. All `go build` output targets `./cmd/jeltz`.
- Not a library; imports freely from `internal/` and `pkg/`.

---

*Structure analysis: 2026-02-24*
