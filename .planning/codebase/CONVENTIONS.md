# Coding Conventions

**Analysis Date:** 2026-02-24

## Naming Patterns

**Files:**
- Use lowercase concise domain names (`internal/proxy/pipeline.go`, `internal/rules/maplocal.go`)
- Use `_test.go` for tests in same package directory (`internal/config/config_test.go`)

**Functions:**
- Exported API in PascalCase (`Load`, `Compile`, `NewPipeline`)
- Internal helpers in camelCase (`applyEnv`, `targetHostPort`, `readRequestBody`)

**Variables:**
- Short scoped names for request-local objects (`fc`, `rs`, `cfg`)
- Descriptive names for config and constants (`UpstreamDialTimeoutMS`, `leafCacheMaxEntries`)

**Types:**
- Domain nouns in PascalCase (`FlowContext`, `RuleSet`, `CLIOverrides`, `MapLocalRule`)

## Code Style

**Formatting:**
- `gofmt` canonical formatting (Go style across all files)

**Linting/Static checks:**
- `go vet` target in `Makefile`
- Explicit `//nolint` annotations used sparingly for intentional cases (for example `InsecureSkipVerify`, unchecked `io.Copy`)

## Import Organization

**Order:**
1. Standard library imports
2. External module imports (`golang.org/x/net/http2`, `gopkg.in/yaml.v3`)
3. Internal module imports (`github.com/fabiant7t/jeltz/...`)

**Path Aliases:**
- No alias path indirection; direct module paths
- Alias used only to disambiguate package names (`pkgca` in `internal/ca/ca.go`)

## Error Handling

**Patterns:**
- Wrap errors with context using `%w` (`fmt.Errorf("...: %w", err)`) in library/runtime code
- Return status-safe fallback responses for request-path failures instead of panicking (`502`, `500`, `403`, `413` paths in `internal/proxy/`)
- Fail fast in CLI startup on invalid config/CA/rules with clear stderr logs (`cmd/jeltz/main.go`)

## Logging

**Framework:** `log/slog`

**Patterns:**
- Create logger once in startup (`internal/logging/logging.go`, `cmd/jeltz/main.go`)
- Use stable key constants from `internal/logging/logging.go` for cross-module consistency
- Log request lifecycle at `Info` and detailed traffic/header dumps at `Debug`

## Comments

**When to Comment:**
- Package comments on every package file (`// Package ...`)
- Comments explain protocol subtleties and ordering constraints (for example MITM ALPN and pipeline steps in `internal/proxy/mitm.go` and `internal/proxy/pipeline.go`)

**GoDoc usage:**
- Exported types/functions include GoDoc-style comments in core packages

## Function Design

**Size:**
- Keep hot-path logic split into helper functions (`Pipeline.Run` delegates to `serveLocal`, `roundtrip`, `readRequestBody`)

**Parameters:**
- Pass explicit context/state structs when flow has many fields (`FlowContext`)
- Keep compile functions input as typed raw config models (`config.RawRule`, `config.RawOps`)

**Return Values:**
- Favor `(*Type, error)` with nil-on-error pattern
- Use sentinel checks via `errors.Is` when semantic distinction is required (`rules.IsTraversal`)

## Module Design

**Exports:**
- `internal/` packages expose only runtime APIs needed by `cmd/jeltz`
- `pkg/` packages expose reusable primitives without disk/runtime coupling (`pkg/ca`, `pkg/p12`, `pkg/xdg`)

**Barrel Files:**
- Not used; Go package exports are consumed directly by package path

---

*Convention analysis: 2026-02-24*
