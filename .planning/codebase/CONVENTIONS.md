# Coding Conventions

**Analysis Date:** 2026-02-24

## Style & Formatting

**Language:** Go (module `github.com/fabiant7t/jeltz`, go 1.25.0)

**Formatting:** Standard `gofmt`/`go vet` — no custom formatter config present. The Makefile exposes `make lint` which runs `go vet ./...`. No `.golangci.yml` or `rustfmt.toml` exists.

**Indentation:** Tabs (Go standard).

**Line length:** No enforced limit; lines stay short by convention (generally under 100 chars).

**Build tags / CGO:** `CGO_ENABLED=0` for all builds (pure Go, no C dependencies).

## Naming Conventions

**Packages:**
- `internal/` packages use simple lowercase names matching their domain: `ca`, `config`, `httpx`, `logging`, `proxy`, `rules`
- `pkg/` packages use the same pattern: `ca`, `p12`, `xdg`
- Package names match their directory names exactly

**Files:**
- Lowercase, no hyphens: `ca.go`, `config.go`, `pipeline.go`, `hopbyhop.go`, `maplocal.go`
- Test files: `<source_file>_test.go` co-located in the same package directory
- Integration tests use a descriptive suffix: `mitm_h2_integration_test.go`

**Types:**
- PascalCase structs: `FlowContext`, `FlowMeta`, `RuleSet`, `HeaderRule`, `DeleteOp`, `SetOp`, `CLIOverrides`
- Interface-like types are plain structs (no Go interfaces used in the codebase)
- Raw/uncompiled config types are prefixed with `Raw`: `RawRule`, `RawMatch`, `RawOps`, `RawDeleteOp`, `RawSetOp`

**Functions and methods:**
- Exported: PascalCase — `CompileMatch`, `CompileOps`, `NewPipeline`, `WriteResponse`
- Unexported: camelCase — `compileHeaderRule`, `compileDeleteOp`, `applyDelete`, `filterHeaderValues`
- Constructors follow `New<Type>` pattern: `NewPipeline`
- Loaders follow `Load` (not `New`) when loading from disk/state: `ca.Load`, `config.Load`
- Compile step for raw config → compiled types: `Compile`, `CompileMatch`, `CompileOps`, `CompileMapLocalRule`

**Variables and constants:**
- Unexported constants: camelCase — `caKeyFile`, `caCertFile`, `validity`
- Exported constants: PascalCase — `P12Password`, `Version`, `RuleTypeHeader`, `RuleTypeMapLocal`
- Logging key constants use `Key` prefix: `KeyComponent`, `KeyEvent`, `KeyError`, `KeyDurationMS`
- ANSI color constants in `cmd/jeltz/banner.go`: `ansiReset`, `ansiBold`, etc.

**CLI flags:** kebab-case: `--listen`, `--config`, `--base-path`, `--data-dir`, `--log-level`, `--insecure-upstream`, `--dump-traffic`, `--max-body-bytes`

**YAML config keys:** snake_case: `base_path`, `data_dir`, `insecure_upstream`, `dump_traffic`, `max_body_bytes`, `any_name`

## Error Handling Pattern

All errors are propagated upward using the `(value, error)` return convention throughout. No `panic` in library code.

**Error wrapping:** `fmt.Errorf("context: %w", err)` is used consistently to add context at each layer:
```go
return nil, fmt.Errorf("rules[%d] (header): %w", i, err)
return nil, fmt.Errorf("match.host regex %q: %w", rm.Host, err)
return nil, fmt.Errorf("ca: create data dir: %w", err)
```

**Fatal errors at the boundary:** `cmd/jeltz/main.go` handles all errors from called functions with `logger.Error(...)` + `os.Exit(1)`. Library code never calls `os.Exit`.

**Non-fatal errors are explicitly noted:** When an error is intentionally ignored (e.g., disk cache write failure), a comment explains why:
```go
// Non-fatal: memory cache still works.
_ = err
```

**`//nolint:errcheck`** is used inline where `io.Copy` and `Close` return values are intentionally discarded (network I/O where errors are unrecoverable at that point). This appears in `internal/proxy/pipeline.go` and `internal/proxy/proxy.go`.

**Sentinel error pattern:** `rules.IsTraversal(err)` is used to detect a specific error type returned by `Resolve`, allowing callers to distinguish traversal errors from other errors without importing the concrete type.

## Code Organization Pattern

**Two-layer package structure:**
- `internal/` — application-specific packages not importable outside the module
- `pkg/` — general-purpose packages (`ca`, `p12`, `xdg`) with no jeltz-specific dependencies; reusable in isolation
- `cmd/jeltz/` — main entry point; only wires things together, no business logic

**Raw → Compiled two-stage design for config/rules:**
- Config is loaded as `Raw*` structs (`RawRule`, `RawMatch`, `RawOps`) via YAML
- A `Compile*` step transforms raw config into validated, ready-to-use types (`Match`, `Ops`, `RuleSet`, `MapLocalRule`)
- Compile functions return errors for invalid input; compiled types are always valid

**File layout within packages:**
- One file per major type/concern: `rules.go` (match), `ruleset.go` (rule set and compile), `headers.go` (header ops), `maplocal.go` (map-local rule)
- Logging keys are centralized in `internal/logging/logging.go` as `const` block with `Key` prefix

**Struct field comments:** Exported struct fields have inline comments explaining non-obvious semantics:
```go
type FlowContext struct {
    Scheme string // "http" or "https"
    Host   string // hostname only, without port
    Body   io.ReadCloser // may be nil
}
```

**Package-level doc comments:** Each package has a `// Package <name> ...` doc comment on the first line of the primary file.

## Notable Patterns

**`t.Helper()` in test helpers:** All test helper functions call `t.Helper()` as their first line so failure messages point to the call site, not the helper body. Examples: `compileOrFatal`, `compileOps`, `makeMapLocalRule`, `writeConfig`.

**`t.TempDir()` for test isolation:** All tests that need the filesystem use `t.TempDir()`, which is automatically cleaned up after the test.

**`t.Setenv()` for environment variable tests:** Used in `pkg/xdg/xdg_test.go` to set and automatically restore environment variables.

**`//nolint` comments are inline and scoped:** Never broad file-level suppression; always line-specific with reason (e.g., `//nolint:errcheck`, `//nolint:gosec`).

**Logging uses `slog` with stable key constants:** All structured log calls use keys from `internal/logging` rather than raw strings:
```go
slog.String(logging.KeyComponent, "main")
slog.String(logging.KeyError, err.Error())
```

**`sync.Mutex` embedded in structs for thread safety:** The `CA` struct uses a `sync.Mutex` field `mu` with explicit `Lock()`/`Unlock()` (not `RWMutex`) to protect a `map[string]*tls.Certificate` cache.

**Context propagation:** `context.Context` is carried in `FlowContext.Ctx` and passed to `http.NewRequestWithContext`. Signal handling in `main.go` uses `signal.NotifyContext`.

**`t.Context()` in integration tests:** Used to obtain the test's context for goroutine cancellation (Go 1.21+).
