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

Status: v0.5. This release extracts real game titles from PE version info
(ProductName → FileDescription, folder-name fallback) so Windows exes get
proper names even on Linux, resolves ProtonDB compatibility tiers
(platinum/gold/silver/bronze/borked) through a title → Steam appid → tier
lookup with per-scan budgets and TTL disk caches, adds scan progress
reporting (phases discover/enrich/covers/lookup, shown as a progress bar in
the GUI and a progress line in the TUI), makes AddDirectory and
ClearBundleCache non-blocking, and fixes the TUI tab bar (a one-line
overflow made bubbletea drop it, hiding the Settings screen). GUI polish:
card buttons fire without opening the detail panel (only card-body clicks
open it), the detail panel is proportional (30% of the window, clamped
300–480px), ProtonDB tier pills on cards and the detail panel, an
online-lookups toggle in Settings, and a dark Wayland CSD titlebar via a
vendored shirei patch (see `docs/vendor-patches.md`). v0.4 shipped
cache-first startup (schema-versioned
`games.json` cache,
status reconciled from store manifests), in-app settings (scan-directory
list with add/remove, launch-template editing), GUI polish (sort menu,
icon view switch, hover states, gradient cover placeholders, empty states
with CTAs, arrow-key grid navigation, raised toasts), and a multi-screen
styled TUI. v0.3 shipped multi-store library scan
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
added folders (recursive); game titles come from PE version info
(ProductName, then FileDescription, then the folder name), so Windows exes
get real titles even on Linux, and Linux scans also accept `.exe` files
without the execute bit;
the grid shows each game's store, installed OptiScaler version, detected
upscaler versions (DLSS/FSR/XeSS marketing names), and ProtonDB tier.
Adding a directory is non-blocking: a placeholder row appears instantly and
is enriched in the background. Launch a game from its
card or the detail panel button (GUI) or the `l` key (TUI): Steam games go through
`steam://rungameid` (Proton is applied by Steam automatically), Epic through
the launcher URL, GOG and manual games via their exe — manual games use the
launch template from Settings. Launching is fire-and-forget; the app never
tracks or kills game processes. Busy install/uninstall operations can be
cancelled per game (Cancel button or `c` in the TUI) and roll back to the
pre-op state.

GUI interactions: the toolbar scans, adds games, filters, sorts
(Default: actionable first / Name: A–Z), and switches between grid and list
views; a progress bar under the toolbar tracks the scan phases
(discover/enrich/covers/lookup). Cards fire their buttons directly — only a
card-body click opens the detail panel, which docks to the right at 30% of
the window width (clamped 300–480px). Arrow keys move the card selection,
Enter opens
the detail side panel, Esc closes it, and Tab/Shift-Tab cycles
focus across every button. ProtonDB tier pills (platinum/gold/silver/bronze/
borked/pending) show on cards and in the detail panel when online lookups
are enabled. Empty libraries and empty filters show guidance
with scan/add/clear-search buttons. On Wayland the client-side titlebar is
dark-themed through a vendored shirei patch (`docs/vendor-patches.md`); X11
keeps the window manager's decorations.

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
launch template, the online game-info toggle, and the clear-cache action.
Scan directories are managed
in-app: "Add directory…" opens the OS directory dialog (zenity or kdialog)
and each row has a Remove button; in the TUI, `a` adds a path and `d`
removes the selected one (with a `y`/`n` confirm). Added games persist and
are scanned recursively. "Add Game" on the GUI toolbar is a shortcut to the
same OS picker. The launch template for manual games
(default `"{exe}" {args}`) is edited in-app: the Launch Template field in
the GUI Settings modal, or `t` on the TUI Settings screen.

The "Online game info (Steam/ProtonDB)" toggle (`online_lookups` in
`settings.json`, default **true**; GUI Settings checkbox, TUI `o`) gates the
lookup phase of each scan: manual games are resolved title → Steam appid
(`steamcommunity.com/actions/SearchApps`) → ProtonDB tier
(`protondb.com` summaries API), and Steam-library rows get their tier
directly by appid. Lookups run under a per-scan budget (8), cache results on
disk (30 days for search, 7 days for summaries), back off on HTTP 429, and
degrade silently when offline. Privacy note: when enabled, game titles are
sent to steamcommunity.com and appids to protondb.com — disable the toggle
for a fully offline scan.

### TUI keymap

| Key | Action |
|-----|--------|
| `1` / `2` / `3` / `4` | Games / Settings / Help / About screens |
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
| Settings: `e` `t` `a` `d` `x` `o` | Edit version / edit launch template / add dir / remove dir (`y`/`n`) / clear bundle cache / toggle online game info |
| Confirm modal | `y` proceed, `n` cancel |

Every screen footer shows the screen-switch hints
(`1 games · 2 settings · 3 help · 4 about`); the About screen shows the
build version and the TUI stack line. During scans a progress line shows
the current phase, bar, and percent.

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
