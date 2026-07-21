# optiscaler-manager

A desktop app that manages [OptiScaler](https://github.com/optiscaler/OptiScaler)
installations for your local games. It downloads the current OptiScaler bundle,
installs it into a game directory with full backup and rollback safety, and
uninstalls cleanly when you're done. Available for **Linux and Windows** (amd64).

## Features

- Scans Steam, Epic, GOG, and manually added folders to build your game library
- Real game titles and cover art, plus ProtonDB compatibility tiers on Linux
- One-click install, uninstall, and rollback, with SHA-verified backups of
  every file it touches
- Detects and adopts OptiScaler setups you installed by hand, so they become
  managed without losing your files
- Launch games straight from the app (Steam, Epic, GOG, or a custom template)
- Both a graphical interface and a terminal UI over the same core
- Open a game's `OptiScaler.ini` for editing right from the app
- Settings for default OptiScaler version, scan directories, launch template,
  and online lookups
- Works offline: everything degrades gracefully with no network, and online
  lookups can be turned off entirely

## Installation

Download the latest release for your platform (Linux or Windows, amd64) from
the [releases page](../../releases), unpack it, and run:

```
optiscaler-manager        # graphical interface
optiscaler-manager tui    # terminal UI
```

## Usage

Scanning covers Steam, Epic, GOG (discovery is Windows-only), and manually
added folders (recursive). A folder that is itself a game gets one row; a
container folder (a library root like `Games` or `Steam`) becomes a scan root
and every game inside it surfaces as its own row. The grid shows each game's
store, installed OptiScaler version, detected upscaler versions
(DLSS/FSR/XeSS), and ProtonDB tier. Launch a game from its card or detail
panel (GUI) or with `l` (TUI); launching is fire-and-forget. Busy installs and
uninstalls can be cancelled per game and roll back to the pre-operation state.

Games with a hand-installed OptiScaler setup show as **external**. The install
action reads **Adopt**: installing backs up the external files SHA-verified
first, so the game becomes managed without losing your setup, and a later
uninstall or rollback restores those files byte-identically.

Your state (manifests, backups, settings, library cache) lives outside game
directories in `~/.local/share/optiscaler-manager`; downloaded bundles and
cover art are cached in `~/.cache/optiscaler-manager`.

### GUI

The toolbar scans, adds games, filters, sorts, and switches between grid and
list views, with a progress bar tracking scan phases. Cards fire their buttons
directly; clicking a card body opens the detail panel. Arrow keys move the
selection, Enter opens the detail panel, Esc closes it. The Settings window
holds the default OptiScaler version, the scan-directory list, the launch
template, the online game-info toggle, and the clear-cache action. The
"Online game info" toggle (on by default) gates Steam/ProtonDB lookups;
turning it off gives you a fully offline scan.

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

### Command line

```
optiscaler-manager                  # launch the GUI
optiscaler-manager tui              # launch the terminal UI
optiscaler-manager gui --audit-grid # raw sortable table view
optiscaler-manager scan             # list installed games + upscalers/versions
optiscaler-manager install <path>   # install OptiScaler into a game directory
optiscaler-manager uninstall <path> # SHA-verified removal
optiscaler-manager rollback <path>  # restore after an interrupted/failed install
optiscaler-manager version
```

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

Interested in the internals or contributing? See [README.dev.md](README.dev.md)
for the architecture, stack, conventions, and release process.

---

## License

[MIT License](LICENSE)
