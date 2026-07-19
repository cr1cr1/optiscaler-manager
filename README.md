# optiscaler-manager

Go desktop app that manages [OptiScaler](https://github.com/optiscaler/OptiScaler)
installations for local games — downloads the current bundle, installs it into
a game directory with full backup + rollback safety, and uninstalls cleanly.

GUI: [go-shirei](https://github.com/hasenj/go-shirei) (`go.hasen.dev/shirei`,
pinned v0.5.2). Built and released for **Linux and Windows** (amd64); the GUI
runs wherever shirei has a backend.

The GUI is a thin [go-shirei](https://github.com/hasenj/go-shirei) binding
over a frontend-agnostic session core (`internal/ui`); the `tui` subcommand
is a second, terminal frontend on the same session.

Status: v0.3 — multi-store library scan (Steam, Epic, GOG, manual folders,
with Windows/macOS discovery probes), per-game upscaler and OptiScaler
versions read from PE resources, one-click game launching (Steam
`steam://rungameid` with automatic Proton, Epic launcher URL, GOG direct exe,
custom template for manual games), a bubbletea TUI, cancellable
install/uninstall operations, and a keyboard-navigable responsive cover-grid
GUI. See `docs/` for scope, architecture, safety model, and the milestone
plan.

## Usage

```
optiscaler-manager                  # launch the GUI
optiscaler-manager tui              # launch the terminal UI (same session core)
optiscaler-manager gui --audit-grid # raw sortable table view
optiscaler-manager scan             # list installed games (all stores) + upscalers/versions
optiscaler-manager install <path>   # install OptiScaler into a game directory
optiscaler-manager uninstall <path> # SHA-verified removal
optiscaler-manager rollback <path>  # restore after an interrupted/failed install
optiscaler-manager version
```

Scanning covers Steam, Epic, GOG, and manually added folders (recursive);
the grid shows each game's store, installed OptiScaler version, and detected
upscaler versions (DLSS/FSR/XeSS marketing names). Launch a game from its
card or dashboard button (GUI) or the `l` key (TUI): Steam games go through
`steam://rungameid` (Proton is applied by Steam automatically), Epic through
the launcher URL, GOG and manual games via their exe — manual games use the
launch template from Settings. Launching is fire-and-forget; the app never
tracks or kills game processes. Busy install/uninstall operations can be
cancelled per game (Cancel button or `c` in the TUI) and roll back to the
pre-op state.

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
dialog (zenity or kdialog) to register any game folder; added games persist
and are scanned recursively. The launch template for manual games
(`launch_template` in `settings.json`, default `"{exe}" {args}`) is edited in
the file directly.

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
