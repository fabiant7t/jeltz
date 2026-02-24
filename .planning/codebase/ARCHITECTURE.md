# Architecture

**Analysis Date:** 2026-02-24

## Pattern Overview

**Overall:** Layered single-binary proxy with rule engine + CA subsystem

**Key Characteristics:**
- Entry-point orchestration in `cmd/jeltz/main.go` wires config, CA, rules, and server
- Request processing split between server transport (`internal/proxy/proxy.go`, `internal/proxy/mitm.go`) and pure pipeline flow (`internal/proxy/pipeline.go`)
- Rule compilation (`internal/rules/*.go`) is separate from request execution (`internal/proxy/pipeline.go`)

## Layers

**CLI/Application Layer:**
- Purpose: parse flags/subcommands, load config, initialize runtime components
- Location: `cmd/jeltz/`
- Contains: startup flow, banner output, CA helper subcommands
- Depends on: `internal/config`, `internal/ca`, `internal/rules`, `internal/proxy`, `pkg/xdg`
- Used by: OS process entrypoint (`main`)

**Config Layer:**
- Purpose: resolve effective settings from defaults, env, file, CLI overrides
- Location: `internal/config/config.go`
- Contains: strict YAML schema, env parsing, precedence merge logic
- Depends on: `gopkg.in/yaml.v3`, stdlib
- Used by: `cmd/jeltz/main.go`

**Proxy Server Layer:**
- Purpose: accept HTTP proxy traffic and dispatch CONNECT vs forward flows
- Location: `internal/proxy/proxy.go`, `internal/proxy/mitm.go`
- Contains: HTTP server lifecycle, tunnel handling, MITM TLS + ALPN selection
- Depends on: pipeline layer + CA interface
- Used by: `cmd/jeltz/main.go`

**Pipeline Layer:**
- Purpose: deterministic request/response transform flow
- Location: `internal/proxy/pipeline.go`
- Contains: request rule application, map-local routing, upstream roundtrip, response rule application
- Depends on: `internal/rules`, `internal/httpx`
- Used by: proxy server layer

**Rules Layer:**
- Purpose: compile and evaluate header/map_local rule definitions
- Location: `internal/rules/`
- Contains: regex matching, header ops, map-local resolution and traversal protection
- Depends on: `internal/config`
- Used by: pipeline and startup compile step

**CA/Crypto Layer:**
- Purpose: create/load CA materials and issue leaf certs
- Location: `internal/ca/`, `pkg/ca/`, `pkg/p12/`
- Contains: persistent root CA management, in-memory leaf cache, PKCS#12 export
- Depends on: stdlib crypto packages
- Used by: MITM handler and CA CLI subcommands

## Data Flow

**Startup Flow:**
1. `cmd/jeltz/main.go` parses CLI flags and subcommand selection
2. `internal/config.Load` resolves defaults + env + YAML + CLI
3. `internal/ca.Load` creates/loads CA files and optional P12 bundle
4. `internal/rules.Compile` compiles YAML rules into runtime `RuleSet`
5. `internal/proxy.New` + `ListenAndServe` starts server

**HTTPS CONNECT MITM Flow:**
1. `internal/proxy/proxy.go` hijacks CONNECT socket
2. `internal/proxy/mitm.go` returns `200 Connection Established`, performs TLS handshake with dynamic leaf from `ca.LeafCert`
3. ALPN branch: HTTP/2 (`serveH2`) or HTTP/1.1 (`serveHTTP1`)
4. Each request converted into `FlowContext` and passed to `Pipeline.Run`
5. Pipeline returns `ResponseResult`; transport layer writes back to client

**Plain HTTP Forward Flow:**
1. Non-CONNECT request enters `handleForward` in `internal/proxy/proxy.go`
2. If pipeline exists, request is transformed to `FlowContext` and processed via `Pipeline.Run`
3. If no pipeline, direct fallback uses `http.DefaultTransport`

**State Management:**
- Runtime mutable state is minimal and scoped: CA cache (`internal/ca/ca.go`), transport pooling (`internal/proxy/pipeline.go`), request-scoped `FlowContext`

## Key Abstractions

**FlowContext / ResponseResult:**
- Purpose: stable request metadata + output contract for pipeline execution
- Examples: `internal/proxy/pipeline.go`
- Pattern: transform-in, result-out pipeline core

**RuleSet:**
- Purpose: compiled immutable rules used at runtime
- Examples: `internal/rules/ruleset.go`, `internal/rules/headers.go`, `internal/rules/maplocal.go`
- Pattern: compile once at startup, evaluate many times per request

**caLoader interface:**
- Purpose: decouple proxy from concrete CA type for testing
- Examples: `internal/proxy/mitm.go`
- Pattern: narrow interface dependency (`LeafCert(host)`)

## Entry Points

**Binary Entry:**
- Location: `cmd/jeltz/main.go`
- Triggers: process launch `jeltz`
- Responsibilities: config, CA, rule compile, logging, server startup, signal-based shutdown

**Subcommands:**
- Location: `cmd/jeltz/main.go`
- Triggers: `jeltz ca-path`, `jeltz ca-p12-path`, `jeltz ca-install-hint`
- Responsibilities: print CA artifact paths and installation guidance

## Error Handling

**Strategy:** return enriched errors from lower layers and log+exit at top-level

**Patterns:**
- Startup failures return wrapped errors (`fmt.Errorf(...: %w)`) and terminate in `cmd/jeltz/main.go`
- Request-time failures prefer HTTP status fallbacks (`500`, `502`, `413`, `403`) in `internal/proxy/pipeline.go` and `internal/proxy/proxy.go`

## Cross-Cutting Concerns

**Logging:** structured `slog` keys centralized in `internal/logging/logging.go`

**Validation:** strict YAML schema (`KnownFields`) + rule compile validation (`internal/config/config.go`, `internal/rules/ruleset.go`)

**Authentication:** not applicable for app users; trust anchored to local CA file set in `internal/ca/ca.go`

---

*Architecture analysis: 2026-02-24*
