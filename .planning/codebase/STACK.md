# Tech Stack

**Analysis Date:** 2026-02-24

## Language(s)

- **Go 1.25.0** — declared in `go.mod`; README states Go 1.26+ as the minimum runtime requirement
  - All source files are `.go`; no secondary languages
  - Module path: `github.com/fabiant7t/jeltz`

## Build System / Package Manager

- **Build tool:** `make` — `Makefile` defines `build`, `test`, `race`, `lint`, `clean`, `release` targets
- **Package manager:** Go modules (`go.mod` + `go.sum`)
- **Lockfile:** `go.sum` present
- **Release tooling:** `goreleaser` v2 config at `.goreleaser.yaml` — produces `tar.gz` archives for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`
- **CGO disabled:** `CGO_ENABLED=0` in both `Makefile` and `.goreleaser.yaml` — fully static binaries
- **Build-time injection:** `main.version`, `main.buildDate`, `main.gitRevision` via `-ldflags`

## Key Dependencies

**Direct (from `go.mod`):**

| Package | Version | Purpose |
|---|---|---|
| `github.com/spf13/viper` | v1.21.0 | Config loading: YAML file + env vars with `JELTZ_` prefix (`internal/config/config.go`) |
| `golang.org/x/net` | v0.50.0 | HTTP/2 server support via `golang.org/x/net/http2` for MITM tunnel leg (`internal/proxy/mitm.go`) |
| `gopkg.in/yaml.v3` | v3.0.1 | Strict YAML config validation with `KnownFields(true)` and rule struct parsing (`internal/config/config.go`) |

**Indirect (selected, all pulled by viper):**

| Package | Version | Notes |
|---|---|---|
| `github.com/fsnotify/fsnotify` | v1.9.0 | File watching (viper dep) |
| `github.com/go-viper/mapstructure/v2` | v2.5.0 | Struct mapping (viper dep) |
| `github.com/pelletier/go-toml/v2` | v2.2.4 | TOML support (viper dep, unused at runtime) |
| `github.com/sagikazarmark/locafero` | v0.12.0 | Filesystem locator (viper dep) |
| `github.com/spf13/afero` | v1.15.0 | Filesystem abstraction (viper dep) |
| `github.com/spf13/cast` | v1.10.0 | Type casting (viper dep) |
| `github.com/spf13/pflag` | v1.0.10 | POSIX flags (viper dep) |
| `github.com/subosito/gotenv` | v1.6.0 | Env loading (viper dep) |
| `golang.org/x/sys` | v0.41.0 | OS syscalls (fsnotify dep) |
| `golang.org/x/text` | v0.34.0 | Text utilities (transitive) |

## Runtime / Platform

- **OS targets:** Linux, macOS (darwin) — amd64 and arm64
- **Deployment:** Single static binary; no daemon manager, container, or runtime service required
- **Config location:** XDG Base Directory Specification — `~/.config/jeltz/` (config), `~/.local/share/jeltz/` (CA/certs)
- **Persistent state:** CA key/cert and per-host leaf certificate cache on local filesystem
- **No external runtime dependencies** — no database, no network service required at startup

## Dev Tooling

- **Linter:** `go vet ./...` via `make lint`
- **Test runner:** `go test ./... -timeout 120s` via `make test`
- **Race detector:** `go test -race ./... -timeout 120s` via `make race`
- **Release automation:** `goreleaser release --clean` via `make release`
- **Test framework:** stdlib `testing` package only — no third-party test framework
- **Logging:** stdlib `log/slog` with `TextHandler` writing structured key=value lines to stderr; key constants defined in `internal/logging/logging.go`
- **CI:** Not detected — no `.github/`, `.gitlab-ci.yml`, or similar CI config in repository

---

*Stack analysis: 2026-02-24*
