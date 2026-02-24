# Roadmap: jeltz

**Created:** 2026-02-24
**Depth:** Quick
**Coverage:** 1/1 requirements mapped

---

## Phases

- [x] **Phase 1: Config Read-Once Refactor** - Consolidate config file I/O to a single `os.ReadFile` call feeding Viper, KnownFields validation, and rule parsing from one shared `[]byte` (completed 2026-02-24)

---

## Phase Details

### Phase 1: Config Read-Once Refactor

**Goal**: The config file is read from disk exactly once; all downstream consumers receive the same in-memory byte slice.

**Depends on**: Nothing (standalone refactor, no external dependencies)

**Requirements**: CONF-01

**Success Criteria** (what must be TRUE):

  1. Starting the proxy with a valid config file produces no second or third `os.ReadFile` / `os.Open` call against the config path during the startup sequence
  2. An invalid YAML config file (unknown field) causes the proxy to exit with a strict-validation error, identical to pre-refactor behaviour
  3. An invalid YAML config file (malformed YAML) causes the proxy to exit with a parse error, identical to pre-refactor behaviour
  4. A valid config file with header rules and `map_local` entries loads correctly and the proxy intercepts traffic according to those rules

**Plans**: 1 plan

Plans:
- [ ] 01-01-PLAN.md — Replace double-read with single os.ReadFile + v.SetConfigType/v.ReadConfig in config.Load

---

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Config Read-Once Refactor | 1/1 | Complete   | 2026-02-24 |

---

*Last updated: 2026-02-24*
