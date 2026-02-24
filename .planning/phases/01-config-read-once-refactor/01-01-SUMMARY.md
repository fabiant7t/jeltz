---
phase: 01-config-read-once-refactor
plan: "01"
subsystem: config
tags: [go, viper, yaml, config]

# Dependency graph
requires: []
provides:
  - "Single-read config.Load: os.ReadFile called exactly once per Load invocation"
  - "Viper fed via v.SetConfigType + v.ReadConfig instead of v.SetConfigFile + v.ReadInConfig"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Single-read pattern: one os.ReadFile into rawYAML []byte, then bytes.NewReader wrapping for each consumer (Viper, yaml decoder, yaml.Unmarshal)"

key-files:
  created: []
  modified:
    - internal/config/config.go

key-decisions:
  - "Use v.SetConfigType('yaml') + v.ReadConfig(bytes.NewReader(rawYAML)) instead of v.SetConfigFile + v.ReadInConfig to allow Viper to consume the already-read bytes"
  - "Preserve the os.Stat guard before os.ReadFile to retain the user-facing 'config file %q not found' error message"

patterns-established:
  - "Single-read pattern: read config bytes once, pass fresh bytes.NewReader to each consumer"

requirements-completed: [CONF-01]

# Metrics
duration: 1min
completed: 2026-02-24
---

# Phase 1 Plan 01: Config Read-Once Refactor Summary

**Eliminated duplicate os.ReadFile in config.Load by replacing v.SetConfigFile+v.ReadInConfig with v.SetConfigType("yaml")+v.ReadConfig(bytes.NewReader(rawYAML)), so the config file is read from disk exactly once per Load call.**

## Performance

- **Duration:** 1 min
- **Started:** 2026-02-24T17:25:36Z
- **Completed:** 2026-02-24T17:26:56Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments
- Removed `v.SetConfigFile(configFile)` and `v.ReadInConfig()` calls from the `if configFile != ""` block
- Replaced the duplicate `os.ReadFile` (previously labelled "re-reading config file for validation") with the primary single read
- Added `v.SetConfigType("yaml")` + `v.ReadConfig(bytes.NewReader(rawYAML))` so Viper consumes the already-read bytes
- All 8 existing tests pass without modification; full test suite exits 0

## Task Commits

Each task was committed atomically:

1. **Task 1: Replace double-read with single os.ReadFile in config.Load** - `601ff8c` (refactor)
2. **Task 2: Run full test suite to confirm no regressions** - verification only, no code changes

**Plan metadata:** (docs commit follows)

## Files Created/Modified
- `internal/config/config.go` - Removed v.SetConfigFile + v.ReadInConfig; added v.SetConfigType("yaml") + v.ReadConfig(bytes.NewReader(rawYAML)); error message changed from "re-reading config file for validation" to "reading config file"

## Before / After: the if-block in config.Load

**Before (double-read):**
```go
v.SetConfigFile(configFile)
if err := v.ReadInConfig(); err != nil {
    return nil, fmt.Errorf("reading config file: %w", err)
}
var readErr error
rawYAML, readErr = os.ReadFile(configFile)
if readErr != nil {
    return nil, fmt.Errorf("re-reading config file for validation: %w", readErr)
}
```

**After (single-read):**
```go
var readErr error
rawYAML, readErr = os.ReadFile(configFile)
if readErr != nil {
    return nil, fmt.Errorf("reading config file: %w", readErr)
}
v.SetConfigType("yaml")
if err := v.ReadConfig(bytes.NewReader(rawYAML)); err != nil {
    return nil, fmt.Errorf("reading config file: %w", err)
}
```

Calls removed: `v.SetConfigFile`, `v.ReadInConfig`, duplicate `os.ReadFile`
Calls added: `v.SetConfigType("yaml")`, `v.ReadConfig(bytes.NewReader(rawYAML))`

## Test Results

All 8 tests passed without modification:
- TestLoad_NoFile - PASS
- TestLoad_BasicYAML - PASS
- TestLoad_UnknownField - PASS
- TestLoad_WrongVersion - PASS
- TestLoad_CLIOverrides - PASS
- TestLoad_RelativeBasePath - PASS
- TestLoad_AbsoluteBasePath - PASS
- TestLoad_MissingConfigFile - PASS

Full test suite (`go test ./...`): all packages PASS, none failed.

## CONF-01 Status

SATISFIED. The config file is opened by the OS exactly once per Load call when a config file path is provided. The single `os.ReadFile` call at line 120 is the only disk read; `rawYAML` is passed as fresh `bytes.NewReader` slices to Viper, the strict KnownFields YAML validator, and `yaml.Unmarshal`.

## Decisions Made
- Used `v.SetConfigType("yaml")` + `v.ReadConfig(bytes.NewReader(rawYAML))` instead of `v.SetConfigFile` + `v.ReadInConfig` — this is the standard Viper pattern for feeding already-read bytes, avoiding an internal file open.
- Preserved the `os.Stat` guard before `os.ReadFile` to retain the user-facing `"config file %q not found"` error (not just an opaque read error).

## Deviations from Plan

None - plan executed exactly as written. The only deviation from the default environment was the Go module cache at `/go/pkg/mod` being read-only; commands were run with `GOMODCACHE=/tmp/gomodcache` to download dependencies to a writable path. This had no effect on the code or tests.

## Issues Encountered
- Go module cache at `/go/pkg/mod` was read-only in the execution environment. Resolved by setting `GOMODCACHE=/tmp/gomodcache` for all go commands. This is an environment-only issue, not a code issue.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- CONF-01 is satisfied; no further work needed for this phase.
- `internal/config/config.go` public API is unchanged; all callers in `cmd/jeltz/main.go` require no updates.

## Self-Check: PASSED

- `internal/config/config.go` — FOUND
- `01-01-SUMMARY.md` — FOUND
- Commit `601ff8c` — FOUND

---
*Phase: 01-config-read-once-refactor*
*Completed: 2026-02-24*
