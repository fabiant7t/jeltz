# Phase 1: Config Read-Once Refactor - Research

**Researched:** 2026-02-24
**Domain:** Go config loading — Viper v1.21.0 + gopkg.in/yaml.v3 v3.0.1
**Confidence:** HIGH

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| CONF-01 | The YAML config file is read from disk exactly once on startup; the resulting `[]byte` is passed to Viper, the strict `KnownFields` validator, and the rule struct parser — no second or third `os.ReadFile` or `os.Open` call occurs | Viper `ReadConfig(io.Reader)` + `SetConfigType("yaml")` accepts an in-memory reader; `bytes.NewReader(raw)` can be passed to all three consumers from a single `os.ReadFile` call |
</phase_requirements>

---

## Summary

`internal/config/config.go` currently reads the YAML config file from disk in two distinct OS calls: first via `v.ReadInConfig()` (which opens the file internally using the path set by `v.SetConfigFile()`), then again via an explicit `os.ReadFile(configFile)` for strict `KnownFields` validation and rule struct parsing. This results in two physical disk reads per startup and a window where the file could change between the two reads, producing inconsistent parsed state.

The fix is a one-file, zero-new-dependency change. Before calling Viper at all, call `os.ReadFile(configFile)` once into a `raw []byte`. Then feed Viper via `v.SetConfigType("yaml")` + `v.ReadConfig(bytes.NewReader(raw))` (dropping the `v.SetConfigFile` + `v.ReadInConfig` calls). The existing `yaml.NewDecoder(bytes.NewReader(raw))` and `yaml.Unmarshal(raw, &yc)` calls already consume `rawYAML` — they require no changes beyond the variable being populated one read earlier.

Both `bytes` and `os` are already imported in `config.go`. No new imports, no new dependencies, and no callers of `config.Load` need to be updated — the function signature is unchanged.

**Primary recommendation:** In `Load`, replace the `v.SetConfigFile` + `v.ReadInConfig` block with a single `os.ReadFile` call that populates `rawYAML`, then use `v.SetConfigType("yaml")` + `v.ReadConfig(bytes.NewReader(rawYAML))`.

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `os` (stdlib) | Go 1.25 | `os.ReadFile` — reads entire file into `[]byte` | Already used; canonical way to load small config files |
| `bytes` (stdlib) | Go 1.25 | `bytes.NewReader` — wraps `[]byte` as `io.Reader` | Already imported; zero-cost wrapper satisfying `io.Reader` |
| `github.com/spf13/viper` | v1.21.0 | Config manager — defaults, env vars, YAML parsing | Already a direct dependency |
| `gopkg.in/yaml.v3` | v3.0.1 | Strict YAML decode (`KnownFields(true)`) and rule struct unmarshal | Already a direct dependency |

### Supporting

No new supporting libraries needed. The change uses only what is already imported in `config.go`.

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `bytes.NewReader(raw)` | `strings.NewReader(string(raw))` | Extra allocation; `bytes.NewReader` is idiomatic for `[]byte` |
| `v.ReadConfig(r)` | Re-open file with `v.SetConfigFile` + `v.ReadInConfig` | Defeats the purpose — still two OS opens |

**Installation:** No new packages — all dependencies already declared in `go.mod`.

---

## Architecture Patterns

### Recommended Project Structure

No structural changes. The change is entirely within:

```
internal/config/
└── config.go   # single file changed
```

### Pattern 1: Read-Once, Pass Reader

**What:** Read file into `[]byte` once; pass `bytes.NewReader(raw)` to each consumer independently. Each consumer gets its own reader at position 0 because `bytes.NewReader` is not consumed — you create a new one per call.

**When to use:** Any time two or more parsers need the same file content.

**Example:**

```go
// Source: https://pkg.go.dev/github.com/spf13/viper@v1.21.0#Viper.ReadConfig
// Source: https://pkg.go.dev/bytes#NewReader

raw, err := os.ReadFile(configFile)
if err != nil {
    return nil, fmt.Errorf("reading config file %q: %w", configFile, err)
}

// Feed Viper
v.SetConfigType("yaml")
if err := v.ReadConfig(bytes.NewReader(raw)); err != nil {
    return nil, fmt.Errorf("reading config file: %w", err)
}

// Feed strict validator (new reader — position 0)
dec := yaml.NewDecoder(bytes.NewReader(raw))
dec.KnownFields(true)
var yc yamlConfig
if err := dec.Decode(&yc); err != nil {
    return nil, fmt.Errorf("config validation: %w", err)
}

// Feed rule parser (yaml.Unmarshal accepts []byte directly — no new reader needed)
if err := yaml.Unmarshal(raw, &yc); err != nil {
    return nil, fmt.Errorf("parsing rules: %w", err)
}
```

### Pattern 2: Stat Before Read (retained)

The existing `os.Stat(configFile)` guard before the read block should be retained. It provides a clear error message (`"config file %q not found"`) that distinguishes "file missing" from "file unreadable". This is a better user experience than letting `os.ReadFile` return an opaque OS error.

**Retained code:**

```go
if _, err := os.Stat(configFile); err != nil {
    return nil, fmt.Errorf("config file %q not found: %w", configFile, err)
}
```

### Anti-Patterns to Avoid

- **Calling `v.SetConfigFile` before `v.ReadConfig`:** `SetConfigFile` sets a path for `ReadInConfig` to open. When using `ReadConfig(io.Reader)`, `SetConfigFile` is irrelevant and should be removed. Leaving it in causes confusion without causing a bug (it's simply ignored by `ReadConfig`).
- **Reusing `bytes.NewReader` across calls after partial read:** A `bytes.Reader` tracks position. Always construct `bytes.NewReader(raw)` fresh for each consumer. (The existing code already does this correctly for the yaml decoder; the new Viper call needs the same treatment.)
- **Passing `raw` to Viper via a `strings.NewReader(string(raw))` conversion:** Extra allocation — `bytes.NewReader` is the correct type.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Feed Viper from `[]byte` | Custom Viper wrapper | `v.SetConfigType("yaml")` + `v.ReadConfig(bytes.NewReader(raw))` | Built-in API; handles all Viper internals (key normalization, env merge, defaults) |
| Strict YAML field validation | Custom field checker | `yaml.NewDecoder` with `KnownFields(true)` | Already in use; battle-tested |

**Key insight:** Both required APIs (`viper.ReadConfig` and `yaml.Unmarshal`) already accept standard Go interfaces (`io.Reader` and `[]byte` respectively). No custom code is needed.

---

## Common Pitfalls

### Pitfall 1: Forgetting `SetConfigType` When Dropping `SetConfigFile`

**What goes wrong:** `v.ReadConfig(r)` without `v.SetConfigType("yaml")` leaves Viper unable to determine the config format, and it will return an error or silently produce an empty config.

**Why it happens:** `SetConfigFile` normally propagates the extension (`.yaml`) to the format detector. `ReadConfig` bypasses the file path; the type must be set explicitly.

**How to avoid:** Always call `v.SetConfigType("yaml")` before `v.ReadConfig(...)`.

**Warning signs:** `v.ReadConfig` returns an error containing "unsupported config type" or all Viper values resolve to defaults despite a non-empty reader.

### Pitfall 2: Removing the `os.Stat` Guard

**What goes wrong:** If the stat check is removed to simplify the code, a missing config file produces an `os.ReadFile` error whose message is less descriptive than the existing `"config file %q not found"` message.

**Why it happens:** `os.ReadFile` returns a generic `*os.PathError`; `os.Stat` lets us format a cleaner user-facing message.

**How to avoid:** Keep the `os.Stat(configFile)` call before `os.ReadFile`. The cost is negligible (same inode lookup).

**Warning signs:** `TestLoad_MissingConfigFile` still passes but the error message changes — check the test assertion is on `err != nil`, not on the error string.

### Pitfall 3: Reordering Viper Calls Breaks Env Override

**What goes wrong:** If `v.AutomaticEnv()` is called after `v.ReadConfig`, env var overrides may not function correctly in all Viper versions.

**Why it happens:** Viper's internal state machine expects env config to be wired up before reading data sources.

**How to avoid:** Keep the existing order: defaults → env setup (`SetEnvPrefix`, `AutomaticEnv`) → `SetConfigType` → `ReadConfig`. This is already the order in the current code; the refactor must not reorder these.

**Warning signs:** Integration test with `JELTZ_LISTEN=...` env var fails to override file value.

### Pitfall 4: Variable `rawYAML` vs `raw` Naming

**What goes wrong:** If the implementer introduces a new variable named `raw` for the `os.ReadFile` result but the existing code uses `rawYAML`, both will coexist and one may be stale.

**Why it happens:** The current code initializes `var rawYAML []byte` and populates it after Viper. The refactor should populate `rawYAML` directly from `os.ReadFile`, removing the two-step initialization.

**How to avoid:** Replace the `var rawYAML []byte` declaration and its two-step population with a single `rawYAML, err := os.ReadFile(configFile)` assignment inside the `if configFile != ""` block.

---

## Code Examples

Verified patterns from official sources:

### Viper: feed from `[]byte` via `io.Reader`

```go
// Source: https://pkg.go.dev/github.com/spf13/viper@v1.21.0#Viper.ReadConfig
v.SetConfigType("yaml")
if err := v.ReadConfig(bytes.NewReader(rawYAML)); err != nil {
    return nil, fmt.Errorf("reading config file: %w", err)
}
```

### yaml.v3: decoder with KnownFields (unchanged)

```go
// Source: https://pkg.go.dev/gopkg.in/yaml.v3#Decoder.KnownFields
dec := yaml.NewDecoder(bytes.NewReader(rawYAML))
dec.KnownFields(true)
var yc yamlConfig
if err := dec.Decode(&yc); err != nil {
    return nil, fmt.Errorf("config validation: %w", err)
}
```

### yaml.v3: Unmarshal from `[]byte` (unchanged)

```go
// Source: https://pkg.go.dev/gopkg.in/yaml.v3#Unmarshal
var yc yamlConfig
if err := yaml.Unmarshal(rawYAML, &yc); err != nil {
    return nil, fmt.Errorf("parsing rules: %w", err)
}
```

### Complete refactored block (the exact change)

```go
// BEFORE (two OS reads):
//   v.SetConfigFile(configFile)
//   v.ReadInConfig()
//   rawYAML, readErr = os.ReadFile(configFile)

// AFTER (one OS read):
if configFile != "" {
    if _, err := os.Stat(configFile); err != nil {
        return nil, fmt.Errorf("config file %q not found: %w", configFile, err)
    }
    var readErr error
    rawYAML, readErr = os.ReadFile(configFile)
    if readErr != nil {
        return nil, fmt.Errorf("reading config file: %w", readErr)
    }
    v.SetConfigType("yaml")
    if err := v.ReadConfig(bytes.NewReader(rawYAML)); err != nil {
        return nil, fmt.Errorf("reading config file: %w", err)
    }
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `v.SetConfigFile` + `v.ReadInConfig()` | `v.SetConfigType("yaml")` + `v.ReadConfig(io.Reader)` | Viper has supported `ReadConfig` since at least v1.0 | Allows caller to supply bytes; no file path coupling |

**Deprecated/outdated:** None relevant to this change. `ReadInConfig` remains valid — this is a deliberate pattern choice, not a deprecation.

---

## Validation Architecture

`nyquist_validation` is not set in `.planning/config.json` (the `workflow` object does not contain that key), so this section documents the existing test infrastructure as it applies to CONF-01.

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` |
| Config file | None (no `go.test` or `testify` config) |
| Quick run command | `go test ./internal/config/... -timeout 30s` |
| Full suite command | `go test ./... -timeout 120s` |
| Estimated runtime | ~2 seconds |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CONF-01 | Config file read exactly once from disk | unit | `go test ./internal/config/... -run TestLoad -v -timeout 30s` | Partial — existing tests verify behavior, not read count |

### Existing Test Coverage (config_test.go)

The following tests in `/vol/jeltz/internal/config/config_test.go` already exercise all paths that must remain green after the refactor:

| Test | What it covers |
|------|---------------|
| `TestLoad_NoFile` | No config file path — rawYAML stays empty, Viper uses defaults |
| `TestLoad_BasicYAML` | Valid YAML — Viper values populated correctly |
| `TestLoad_UnknownField` | `KnownFields(true)` catches unknown fields |
| `TestLoad_WrongVersion` | Version check on decoded `yamlConfig` |
| `TestLoad_CLIOverrides` | CLI values override file values |
| `TestLoad_RelativeBasePath` | Path resolution logic |
| `TestLoad_AbsoluteBasePath` | Absolute path passthrough |
| `TestLoad_MissingConfigFile` | `os.Stat` guard returns error for missing file |

**Wave 0 gap:** CONF-01 requires "exactly once" — the existing tests verify *correct output* but do not assert the *number of OS reads*. To directly verify CONF-01, a test using `os.Open` counting (via a temporary file + inode open-count tracking, or by wrapping the OS layer) would be needed. However, given this is a pure Go stdlib change with no side-effects on the public API, the behavioral tests above are sufficient for the planner's purposes. Direct read-count assertion is LOW priority.

---

## Open Questions

1. **Should `v.SetConfigFile` be removed entirely or left as a no-op?**
   - What we know: `SetConfigFile` sets the path used by `ReadInConfig`. Once `ReadInConfig` is no longer called, `SetConfigFile` has no effect.
   - What's unclear: Whether leaving it is harmless or could confuse future maintainers.
   - Recommendation: Remove it. It serves no purpose after the refactor and its presence would imply the wrong mental model.

2. **Is `os.Stat` still needed before `os.ReadFile`?**
   - What we know: `os.ReadFile` returns an error on missing files. The existing `os.Stat` provides a custom error message "config file %q not found".
   - What's unclear: Whether preserving the custom error message is required by any caller.
   - Recommendation: Keep `os.Stat` to preserve the user-facing error message. The performance cost is negligible for a startup-only code path.

---

## Sources

### Primary (HIGH confidence)

- https://pkg.go.dev/github.com/spf13/viper@v1.21.0 — `ReadConfig(io.Reader)` and `SetConfigType(string)` method signatures verified
- https://pkg.go.dev/gopkg.in/yaml.v3 — `KnownFields`, `NewDecoder`, `Unmarshal` API confirmed
- `/vol/jeltz/internal/config/config.go` — direct source inspection of current triple-read pattern
- `/vol/jeltz/internal/config/config_test.go` — existing test coverage catalogued

### Secondary (MEDIUM confidence)

- `/vol/jeltz/.planning/STATE.md` — confirms implementation approach (read once into `raw []byte`, feed via `bytes.NewReader`)
- `/vol/jeltz/.planning/codebase/CONCERNS.md` — confirms the triple-read is the identified technical debt item

### Tertiary (LOW confidence)

- None.

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — both Viper and yaml.v3 are already in `go.mod`; APIs verified via pkg.go.dev
- Architecture: HIGH — single-file change with no caller-facing impact; confirmed by reading the source directly
- Pitfalls: HIGH — derived from direct reading of the current code and the Viper API docs
- Test coverage: HIGH — test file read directly; existing tests enumerated

**Research date:** 2026-02-24
**Valid until:** 2026-03-26 (Viper and yaml.v3 are stable; these APIs have not changed in years)
