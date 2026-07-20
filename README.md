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

Status: v0.4. This release adds cache-first startup (schema-versioned
`games.json` cache,
status reconciled from store manifests), in-app settings (scan-directory
list with add/remove, launch-template editing), GUI polish (right-docked
detail panel, sort menu, icon view switch, hover states, gradient cover
placeholders, empty states with CTAs, arrow-key grid navigation, raised
toasts), and a multi-screen styled TUI (screens, detail view, live filter,
settings directory management). v0.3 shipped multi-store library scan
(Steam, Epic, GOG, manual folders, with Windows/macOS discovery probes),
per-game upscaler and OptiScaler versions read from PE resources, one-click
game launching (Steam `steam://rungameid` with automatic Proton, Epic
launcher URL, GOG direct exe, custom template for manual games), a bubbletea
TUI, cancellable install/uninstall operations, and a keyboard-navigable
responsive cover-grid GUI. See `docs/` for scope, architecture, safety
model, and the milestone plan.

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

Scanning covers Steam, Epic, GOG (discovery is Windows-only), and manually
added folders (recursive);
the grid shows each game's store, installed OptiScaler version, and detected
upscaler versions (DLSS/FSR/XeSS marketing names). Launch a game from its
card or the detail panel button (GUI) or the `l` key (TUI): Steam games go through
`steam://rungameid` (Proton is applied by Steam automatically), Epic through
the launcher URL, GOG and manual games via their exe — manual games use the
launch template from Settings. Launching is fire-and-forget; the app never
tracks or kills game processes. Busy install/uninstall operations can be
cancelled per game (Cancel button or `c` in the TUI) and roll back to the
pre-op state.

GUI interactions: the toolbar scans, adds games, filters, sorts
(Default: actionable first / Name: A–Z), and switches between grid and list
views; scan runs with a spinner and the sort/search/view controls disable
while the library is empty. Arrow keys move the card selection, Enter opens
the right-docked detail side panel, Esc closes it, and Tab/Shift-Tab cycles
focus across every button. Empty libraries and empty filters show guidance
with scan/add/clear-search buttons.

Installing into an EAC-protected game (`start_protected_game.exe`) is refused
unless `--force` is passed (GUI asks instead). When the GitHub API is
rate-limited, stale cached release info is refused unless `--allow-cached` is
passed.

State (manifests, backups, `settings.json`, the `games.json` library cache)
lives outside game directories in
`$XDG_DATA_HOME/optiscaler-manager` (default `~/.local/share/optiscaler-manager`).
Startup reads `games.json` first: a warm cache shows the library instantly
without scanning (status `N games (cached)`, with each game's install status
reconciled from the store manifests); a missing or corrupt cache falls
through to a full scan. The cache is rewritten after every scan, directory
add/remove, and completed operation. Rescan explicitly with the Scan button
(GUI) or `R` (TUI).
OptiScaler bundles and cover art are cached in
`$XDG_CACHE_HOME/optiscaler-manager` (default `~/.cache/optiscaler-manager`) —
bundles at `optiscaler/<version>/` are reused before any download and can be
cleared from Settings.

The Settings window (GUI sidebar, TUI screen `2`) holds the default
OptiScaler version (`latest` or a tag), the scan-directories list, the
launch template, and the clear-cache action. Scan directories are managed
in-app: "Add directory…" opens the OS directory dialog (zenity or kdialog)
and each row has a Remove button; in the TUI, `a` adds a path and `d`
removes the selected one (with a `y`/`n` confirm). Added games persist and
are scanned recursively. "Add Game" on the GUI toolbar is a shortcut to the
same OS picker. The launch template for manual games
(default `"{exe}" {args}`) is edited in-app: the Launch Template field in
the GUI Settings modal, or `t` on the TUI Settings screen.

### TUI keymap

| Key | Action |
|-----|--------|
| `1` / `2` / `3` | Games / Settings / Help screens |
| `q`, `ctrl+c` | Quit |
| `j`/`k` or `↓`/`↑` | Move cursor |
| `enter` | Open the detail screen (`esc` back) |
| `i` | Install / uninstall (quick toggle) |
| `l` | Launch game |
| `c` | Cancel the busy operation |
| `/` | Filter, live as you type (`esc` clears) |
| `s` | Toggle sort (default / name) |
| `R` | Rescan the library |
| Detail: `i` `l` `c` `r` `o` | Install / launch / cancel / rollback / open OptiScaler.ini |
| Settings: `e` `t` `a` `d` `x` | Edit version / edit launch template / add dir / remove dir (`y`/`n`) / clear bundle cache |
| Confirm modal | `y` proceed, `n` cancel |

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
