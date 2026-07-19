# optiscaler-manager

Go desktop app that manages [OptiScaler](https://github.com/optiscaler/OptiScaler)
installations for local games — downloads the current bundle, installs it into
a game directory with full backup + rollback safety, and uninstalls cleanly.

GUI: [go-shirei](https://github.com/hasenj/go-shirei) (`go.hasen.dev/shirei`,
pinned v0.5.2). v0.1 targets **Linux + Steam (Proton)**.

The GUI is a thin [go-shirei](https://github.com/hasenj/go-shirei) binding
over a frontend-agnostic session core (`internal/ui`) — a bubbletea TUI can
bind to the same session later.

Status: v0.2 — cover-card grid GUI (dark theme, sidebar, toasts, status bar,
quick install/uninstall per game), headless CLI, and the interactive session
core shared by future frontends. See `docs/` for scope, architecture, safety
model, and the milestone plan.

## Usage

```
optiscaler-manager                  # launch the GUI
optiscaler-manager gui --audit-grid # raw sortable table view
optiscaler-manager scan             # list installed games (all stores) + upscalers/versions
optiscaler-manager install <path>   # install OptiScaler into a game directory
optiscaler-manager uninstall <path> # SHA-verified removal
optiscaler-manager rollback <path>  # restore after an interrupted/failed install
optiscaler-manager version
```

Installing into an EAC-protected game (`start_protected_game.exe`) is refused
unless `--force` is passed (GUI asks instead). When the GitHub API is
rate-limited, stale cached release info is refused unless `--allow-cached` is
passed.

State (manifests, backups) lives outside game directories in
`$XDG_DATA_HOME/optiscaler-manager` (default `~/.local/share/optiscaler-manager`).
OptiScaler bundles and cover art are cached in
`$XDG_CACHE_HOME/optiscaler-manager` (default `~/.cache/optiscaler-manager`) —
bundles at `optiscaler/<version>/` are reused before any download and can be
cleared from Settings.

The Settings window (sidebar) holds the default OptiScaler version (`latest`
or a tag) and the clear-cache action. "Add Game" opens the OS directory
dialog (zenity or kdialog) to register any game folder; added games persist.

### Environment variables

| Variable | Effect |
|----------|--------|
| `OM_DATA_DIR` | Override the state root (manifests, backups, settings) |
| `OM_CACHE_DIR` | Override the cache root (bundles, covers) |
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
