# optiscaler-manager

Go desktop app that manages [OptiScaler](https://github.com/optiscaler/OptiScaler)
installations for local games — downloads the current bundle, installs it into
a game directory with full backup + rollback safety, and uninstalls cleanly.

GUI: [go-shirei](https://github.com/hasenj/go-shirei) (`go.hasen.dev/shirei`,
pinned v0.5.2). v0.1 targets **Linux + Steam (Proton)**.

Status: v0.1 feature-complete. See `docs/` for scope, architecture, safety
model, and the milestone plan.

## Usage

```
optiscaler-manager                  # launch the GUI
optiscaler-manager gui --audit-grid # raw sortable table view
optiscaler-manager scan             # list installed Steam games + upscalers
optiscaler-manager install <path>   # install OptiScaler into a game directory
optiscaler-manager uninstall <path> # SHA-verified removal
optiscaler-manager rollback <path>  # restore after an interrupted/failed install
optiscaler-manager version
```

Installing into an EAC-protected game (`start_protected_game.exe`) is refused
unless `--force` is passed (GUI asks instead). When the GitHub API is
rate-limited, stale cached release info is refused unless `--allow-cached` is
passed.

State (manifests, backups, download cache) lives outside game directories in
`$XDG_DATA_HOME/optiscaler-manager` (default `~/.local/share/optiscaler-manager`).

### Environment variables

| Variable | Effect |
|----------|--------|
| `OM_DATA_DIR` | Override the state root (manifests, backups, cache) |
| `OM_STEAM_ROOT` | Scan only this Steam root |
| `OM_GH_BASE_URL` | Override the GitHub API base URL |
| `OM_LOG_LEVEL` / `OM_LOG_FORMAT` | zerolog level / console\|json |
| `OM_TEST_ARCHIVE` | Point the archive spike test at a real bundle .7z |

## Development

```
go test ./...        # full verification (the only sanctioned test entrypoint)
go vet ./...
golangci-lint run
```

Conventions: TDD first, zerolog in production code, `t.Log` in tests,
OKF-formatted docs under `docs/`, vendored dependencies, commit per completed
task. See `AGENTS.md` and `docs/`.

---

## License

[MIT License](LICENSE)
