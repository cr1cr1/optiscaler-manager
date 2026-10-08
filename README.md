# optiscaler-manager

A desktop app that manages [OptiScaler](https://github.com/optiscaler/OptiScaler)
installations for your local games. It downloads the current OptiScaler bundle,
installs it into a game directory with full backup and rollback safety, and
uninstalls cleanly when you're done. Available for **Linux and Windows** (amd64).

## Features

- Scans Steam, Epic, GOG (including Heroic Games Launcher installs on
  Linux), and manually added folders to build your game library
- Real game titles and cover art, plus ProtonDB compatibility tiers on Linux
- One-click install, uninstall, and rollback, with SHA-verified backups of
  every file it touches, written into the game directory itself. Any
  backup over 100 MB asks for your confirmation before a byte is touched
  (non-interactive CLI runs refuse instead)
- Detects and adopts OptiScaler setups you installed by hand, so they become
  managed without losing your files
- Update NVIDIA DLSS from the official
  [NVIDIA/DLSS](https://github.com/NVIDIA/DLSS) repository on demand: press
  the DLSS version label (GUI) or `u` (TUI) to fetch `nvngx_dlss.dll`,
  `nvngx_dlssd.dll`, and `nvngx_dlssg.dll` from one pinned source commit,
  with a hash-verified backup of your current set in the game directory
  (skipped with a warning
  when that set is incomplete — the update still installs the missing
  DLLs) and a restore menu
  (`▼` in the GUI, `p` on the TUI detail screen) — the app ships no
  NVIDIA files itself. Downloads are cached per source commit and reused
  while that commit stays current.
- Disable or enable a managed install on demand: the injection hook is
  renamed out of the way (`.disabled` or any backup suffix you chose)
  instead of removed, so the toggle back is a single rename
- Launch games straight from the app (Steam, Epic, GOG, or a custom template;
  on Linux, manually added or Heroic-installed Windows binaries can run
  through [umu-launcher](https://github.com/Open-Wine-Components/umu-launcher)
  when installed)
- Both a graphical interface and a terminal UI over the same core
- Open a game's `OptiScaler.ini` for editing right from the app
- Settings for default OptiScaler version, scan directories, launch template,
  online lookups, and card size
- Selectable OptiScaler distribution: install from upstream or a fork —
  [DLSSNR-PreSR-Multipass](https://github.com/jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass)
  is bundled as the first alternative, and you can add your own
  (owner/repo + asset glob) or remove them in Settings
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

Scanning covers Steam, Epic, GOG, and manually
added folders (recursive). Native Epic/GOG discovery is Windows-only; on
Linux those games are found through Heroic Games Launcher's installed
records (native and Flatpak installs). A folder that is itself a game gets
one row; a
container folder (a library root like `Games` or `Steam`) becomes a scan root
and every game inside it surfaces as its own row. Cards render as they are
discovered (the scan never blocks the UI, and only one scan runs at a time);
cover art pops in as it resolves. The grid shows each game's
store, installed OptiScaler version, detected upscaler versions
(DLSS/FSR/XeSS), and ProtonDB tier. Launch a game from its card or detail
panel (GUI) or with `l` (TUI); launching is fire-and-forget. Busy installs and
uninstalls can be cancelled per game and roll back to the pre-operation state.

Cover art comes from Steam's CDN first, then PCGamingWiki box art
(keyless), then Wikidata/Commons box art (keyless — the only automatic
source for console titles with no PC store page), then SteamGridDB when
a key is configured. Name-based lookups are junk-tolerant: repack tags
and edition suffixes in folder names ("Spelunky HD", "… PROPER") are
stripped or truncated away until the store answers. SteamGridDB covers
games Steam has no art for (unreleased or asset-less appids) and resolves
folder names that drop a subtitle ("The Witcher 3 Remastered" → The
Witcher 3: Wild Hunt). Whatever the source — including landscape hero
banners, the last-resort fallback — every cached cover is center-cropped
to the 2:3 poster aspect, so cards never show stretched art. To enable it, create a free API key at
[steamgriddb.com](https://www.steamgriddb.com/profile/preferences/api)
and add it to `~/.local/share/optiscaler-manager/settings.json`:

```json
"steamgriddb_key": "your-key-here"
```

When automatic identification still misses — a GOG edition title Steam's
search rejects, junk PE metadata ("ControlLauncher"), or a game that
isn't on Steam at all — the detail panel has two manual fixes (GUI
buttons; `t` / `a` / `A` keys in the TUI):

- **Set title** pins the display title (persisted as
  `"title_overrides"` in the same settings file); the cover re-resolves
  against the new title immediately. An empty title clears the pin.
- **Set poster** uploads your own image (native file dialog in the GUI,
  typed path in the TUI). It is copied into the cover cache, cropped to
  the 2:3 poster aspect like any fetched art, and beats every automatic
  source — it survives rescans until **Reset poster** removes it
  (persisted as `"cover_overrides"`).

Games in unusual layouts are found without any manual help in most
cases: engine/build folders (`bin`, `Binaries/Win64`, UE packaged-build
staging like `IntermediateBuildDRM/WindowsNoEditor`) are transparent —
their executables count as the parent game's, never as separate rows —
the executable search descends five levels, and
installer/updater/helper binaries (including launchers and `elevate.exe`
shims) never count as the game.

Both pins can also be JSON-edited directly, e.g.
`"title_overrides": {"/games/witcher3": "The Witcher 3: Wild Hunt"}`,
then rescan.

Each installed game manages its own OptiScaler version: the version selector
(a dropdown on the card and detail panel in the GUI, the `v` key in the TUI)
offers the versions already downloaded in the bundle cache plus the default
version from preferences, and switching installs the chosen version while
keeping the game's existing `OptiScaler.ini` tweaks. On startup the app
resolves the newest release once and the selector offers it as a named
`Latest (tag)` row — the first dropdown row (GUI), in the TUI `v` cycle
— picking it installs the latest, re-resolved at pick time. The concrete
cached versions are listed alongside it and never duplicated: wherever
the list already carries the latest tag, one absorbed row replaces it.
While a Latest row is staged, the TUI games cell shows a short `→ Latest` (the detail
line carries the full tag; a mid-tag cut like `→ Latest (v0.9…` would
read as version v0.9).

The DLSS version label is also a control (GUI only): pressing it updates the
game's NVIDIA runtime — all three DLLs (`nvngx_dlss.dll`, `nvngx_dlssd.dll`,
`nvngx_dlssg.dll`) together, from the official NVIDIA/DLSS repository at one
pinned commit. All three must already exist: the updater never adds
components a game did not ship. A DLL whose version resource does not
parse still shows a pressable bare `DLSS` pill. On startup (online
lookups enabled) the app makes one GitHub tags call to learn the
published version — it is shown in the TUI detail view and marks the
update label when your set is older — and pre-downloads that latest
published set into the download cache when it is not already cached, so
a press is cache-first from the first moment. The startup check also
pre-downloads the latest OptiScaler bundle into the cache (the `Latest`
dropdown row above). Pressing the label is cache-first: if the published
commit is already in
the download cache it installs from there, otherwise the commit is
resolved and fetched. Pressing when the game already holds the target
set (or a newer one) is a graceful no-op — it reports the installed
version and writes nothing. Backups deduplicate: a set whose files an
existing backup already holds is reused instead of copied again.
Every update and restore first writes a
hash-verified backup of your current set into the game directory itself
(`dlss-backups/<timestamp>_dlss-<version>/`, next to the DLLs it
protects — it travels with the game folder and can be restored by hand);
the ▼ button beside the label
lists those backups, and restoring one asks for confirmation first.
Downloaded NVIDIA files are subject to NVIDIA's RTX SDK license. Like the
OptiScaler bundles, they are cached per source commit under the app's
cache directory and reused while the commit stays current (a moved `main`
fetches the new commit); every cached file is re-checked against the
cache manifest — SHA-256 mismatch or failed PE validation means it is
fetched again, never installed.

Games with a hand-installed OptiScaler setup show as **external**. The install
action reads **Adopt**: installing backs up the external files SHA-verified
first, so the game becomes managed without losing your setup, and a later
uninstall or rollback restores those files byte-identically.

Selecting a game (clicking a card or row, opening the TUI detail screen)
re-checks its OptiScaler state on disk, so files added, removed, or renamed
by hand between scans show up without a rescan.

Your state (manifests, settings, library cache) lives outside game
directories in `~/.local/share/optiscaler-manager`; downloaded bundles and
cover art are cached in `~/.cache/optiscaler-manager`. Backups are the
exception: every original file the app replaces is backed up inside the
game directory (`optiscaler-backups/` for OptiScaler installs,
`dlss-backups/` for the NVIDIA runtime), next to the files it protects.

### GUI

The toolbar scans, adds games, filters, sorts, and switches between grid and
list views, with a progress bar tracking scan phases. Cards fire their buttons
directly; clicking a card body opens the detail panel. An installed game's
version pill is a dropdown: pick the `Latest (tag)` row (the newest release,
resolved at startup and re-resolved at pick time) or any cached bundle
version to switch to it, keeping your `OptiScaler.ini`.
Arrow keys move the
selection, Enter opens the detail panel, Esc closes it. The Settings window
is split into two tabs. General holds the card size, the scan-directory
list, the launch template, and the online game-info toggle. Optiscaler
holds the default OptiScaler version, the OptiScaler sources (pick the
active distribution fork, or add your own with owner/repo + asset glob —
a new source becomes active on add), and the clear-cache action.
Installed games get a Disable/Enable button in the detail panel: it parks
the injection hook by renaming it — `.disabled`, or the backup name you
chose if you renamed it by hand — instead of removing it, so the game
stops loading OptiScaler until you toggle it back. A parked install shows
a "disabled" badge on its card and in the detail panel. The
"Online game info" toggle (on by default) gates Steam/ProtonDB lookups;
turning it off gives you a fully offline scan. On Linux, the Settings
window also exposes the umu-launcher integration: enable it to launch
manually-added Windows binaries (`.exe`, `.bat`, `.cmd`, `.msi`) via
[umu-launcher](https://github.com/Open-Wine-Components/umu-launcher) when
`umu-run` is installed. Each game gets its own Proton prefix under
`~/.local/share/optiscaler-manager/umu-prefixes/<slug>`. Pin a Proton
build in Settings, or leave it blank to auto-detect from Steam
`compatibilitytools.d`, Bottles `runners/`, and umu `compatibilitytools`
(falling through to umu's `UMU-Latest` on first launch).

### TUI keymap

| Key | Action |
|-----|--------|
| `1` / `2` / `3` / `4` | Games / Settings / Help / About screens |
| `q`, `ctrl+c` | Quit |
| `j`/`k` or `↓`/`↑` | Move cursor |
| `enter` | Open the detail screen (`esc` back) |
| `i` | Install / uninstall (quick toggle) |
| `v` | Switch OptiScaler version (cycle candidates, `enter` confirm, `esc` cancel) |
| `u` | Update the NVIDIA DLSS set from the official repository |
| `l` | Launch game |
| `c` | Cancel the busy operation |
| `/` | Filter, live as you type (`esc` clears) |
| `s` | Toggle sort (default / name) |
| `R` | Rescan the library |
| Detail: `i` `v` `u` `p` `l` `c` `r` `o` `d` | Install / switch version / update the NVIDIA DLSS set / restore a DLSS backup (`p` opens the backup list, `enter` picks, then `y` confirm) / launch / cancel / rollback / open OptiScaler.ini / disable-enable the OptiScaler hook |
| Settings: `tab` `enter` `e` `t` `a` `d` `x` `o` `u` `p` | Switch sources/dirs list / use fork / edit version / edit launch template / add dir or fork / remove dir or fork (`y`/`n`) / clear bundle cache / toggle online game info / toggle umu-launcher / edit umu Proton path |
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
optiscaler-manager switch <path> [--version <tag|latest>] # switch version (default: preferences default)
optiscaler-manager dlss-update <path>   # update the NVIDIA DLSS runtime set
optiscaler-manager dlss-restore <path> [--snapshot <id>] # restore a DLSS backup (default: newest)
optiscaler-manager launch <path>        # fire-and-forget launch request
optiscaler-manager hook <path> --enable|--disable # park/un-park the hook DLL
optiscaler-manager version
```

The session-backed ops (`switch`, `dlss-update`, `dlss-restore`,
`launch`, `hook`) run the same core as the GUI/TUI and honor the same
consent model: gates prompt `y/n` on the terminal, and a non-interactive
stdin declines — there is no `--yes` flag. `--timeout` (default 10m)
bounds every wait (the synchronous `hook` toggle has none); failures
exit 1, usage errors exit 2.

### Environment variables

| Variable | Effect |
|----------|--------|
| `OM_DATA_DIR` | Override the state root (manifests, settings, library cache) |
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
