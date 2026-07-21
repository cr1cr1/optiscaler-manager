# optiscaler-manager: developer guide

Technical details for contributors. For user documentation, see
[README.md](README.md). Project scope, architecture, safety model, and the
milestone plan live under `docs/`; release history is tracked in
`docs/log.md`.

## Architecture

The GUI is a thin [go-shirei](https://github.com/hasenj/go-shirei) binding
(`go.hasen.dev/shirei`, pinned v0.5.2, vendored) over a frontend-agnostic
session core in `internal/ui`; the `tui` subcommand is a second, terminal
frontend on the same session. All behavior lives in the session core, so
frontends stay dumb.

State lives outside game directories:

- `$XDG_DATA_HOME/optiscaler-manager` (default
  `~/.local/share/optiscaler-manager`): store manifests, SHA-verified backups,
  `settings.json`, and the schema-versioned `games.json` library cache.
  Startup reads `games.json` first and reconciles install status from the
  store manifests; a missing or corrupt cache falls through to a full scan.
- `$XDG_CACHE_HOME/optiscaler-manager` (default
  `~/.cache/optiscaler-manager`): OptiScaler bundles at
  `optiscaler/<version>/` (reused before any download) and cover art.

See `docs/architecture.md` for the full picture and `docs/safety.md` for the
backup/rollback model.

## Stack

- Go, with [shirei](https://github.com/hasenj/go-shirei) for the GUI and
  [bubbletea](https://github.com/charmbracelet/bubbletea) for the TUI
- [kong](https://github.com/alecthomas/kong) for CLI parsing
- sevenzip for bundle extraction
- [zerolog](https://github.com/rs/zerolog) for logging

## Development

```
go test ./...        # full verification (the only sanctioned test entrypoint)
go vet ./...
golangci-lint run
```

Conventions:

- TDD first: write or extend a failing test before production code
- `go test ./...` is the only sanctioned test entrypoint; never `go test -run`
  to skip tests
- zerolog in production code, `t.Log` in tests
- Vendored dependencies; local patches to vendored code are documented in
  `docs/vendor-patches.md`
- OKF-formatted docs under `docs/`, kept current (including `docs/log.md`)
- One commit per completed task, after all tests pass

See `AGENTS.md` for the full agent/contributor rules.

## Release process

Releases are built with [goreleaser](https://goreleaser.com) for Linux and
Windows (amd64). Use `goreleaser release --snapshot` for local builds. The
version is injected at build time via ldflags (`-X=main.version=<tag>_<commit>`)
from the git tag; snapshot builds use a `-next` suffix.
