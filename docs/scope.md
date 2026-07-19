---
type: reference
---

# Scope

v0.1 scope as settled by adversarial planning (3 rounds, 2026-07-19). Decisions
here are closed; reopen only with new evidence.

## Platform

- **Linux + Steam (Proton) only.** All other launchers' detection mechanisms are
  Windows-registry-based; the user's platform is Linux. Windows support and other
  launchers are deferred, not abandoned.
- Games run under Proton; OptiScaler files still install into the game directory
  (community practice). Proton prefixes live at `steamapps/compatdata/<appid>/pfx`.

## Component model: bundle-only

- OptiScaler ≥ 0.9 ships **one `.7z` asset** bundling fakenvapi, NukemFG
  (dlssg-to-fsr3), and FFX/XeSS SDK DLLs. There is no component registry, no
  beta channel, no version mix-and-match in v0.1.
- Release asset resolved by **glob `Optiscaler_*.7z`**, never exact filename
  (names embed a date and a `_MM` marker).
- After extraction, a **required-file set is validated**; mismatch fails loudly.
- The separate upstream downloads the reference C# client uses are stale:
  Nukem9/dlssg-to-fsr3 ≥ 0.130 has no GitHub assets (moved to Nexus Mods);
  OptiPatcher is a raw `.asi` and out of scope.

## Install

- Copy-based, never symlink. `OptiScaler.dll` renamed to the injection DLL —
  `dxgi.dll` by default (alternates: winmm, d3d12, dbghelp, version, wininet,
  winhttp — config later).
- Install-dir resolution: UE5 `Phoenix\Binaries\Win64` rule first, else simple
  exe scoring; skip crash/redist/setup/launcher executables.
- Curated safe-defaults `OptiScaler.ini` written on install. "Open in system
  editor" affordance. **No in-app INI editor, no profiles, no import.**
- Anti-cheat: `start_protected_game.exe` exists-check → warning modal before
  install.

## Discovery & classification

- Steam only: `libraryfolders.vdf` + `appmanifest_*.acf` (parsed with
  `github.com/lewisgibson/go-vdf`).
- Classifier reports upscaler **kind + DLL filename only** (DLSS
  `nvngx_dlss.dll`, DLSS-FG `nvngx_dlssg.dll`, FSR `amd_fidelityfx_*`/`ffx_*`,
  XeSS `libxess.dll`). **PE version display is cut**: `debug/pe` has no
  version-resource API; a hand-rolled `FEEF04BD` resource scan is deferred.

## UX

- `optiscaler-manager` with no args launches the GUI (kong
  `default:"withargs"`). Headless subcommands: `scan`, `install <path>`,
  `uninstall <path>`.
- "Action List": single window, fuzzy filter over a virtualized list;
  unfiltered view sorts actionable items first (failed installs, updates),
  then recency. Per-game dashboard with progress, EAC modal, install/uninstall.
  Hidden `--audit-grid` flag dumps the raw table.

## Cut list (deferred)

Windows builds; Epic/GOG/Xbox/EA/Ubisoft/Battle.net/Lutris/Heroic scanners;
custom folders; manual-exe GUI add; PE version display; cover art / SteamGridDB;
grid view; profile system and INI editors; import-from-INI; bulk install;
self-update; i18n; GPU detection; analysis cache; clean-folder tool; beta
channel; component pickers.

## v0.2 scope (GUI restyle + frontend abstraction)

Added after v0.1, modeled on the reference client's main window:

- **Cover-art card grid** (default view) with list view toggle. Cards:
  cover, platform pill, installed badge, EAC badge, status badges, tech
  pills, quick-install toggle.
- **Covers**: Steam CDN `library_600x900.jpg` by appid (primary) → Steam
  store search (name→appid, zero-key fallback) → generated placeholder.
  Cached on disk by sanitized appid. (Ecosystem-verified keyless pattern:
  Lutris and Heroic use the same Steam CDN primary; Bottles uses a private
  SteamGridDB proxy, not copyable.)
- **Bundle cache**: OptiScaler bundles at
  `$XDG_CACHE_HOME/optiscaler-manager/optiscaler/<version>/` (default
  `~/.cache/...`), reused before any download (`OM_CACHE_DIR` overrides).
- **Settings** (persisted `settings.json` in the data root): default
  OptiScaler version (tag or `latest`), manually added game directories;
  settings window with version input and clear-cache action.
- **Manual game add**: OS directory dialog (zenity → kdialog) via
  `internal/pickdir`; added dirs persist and survive rescans.
- **Chrome**: dark theme (incl. dark modal cards — upstream Modal is
  hardcoded white, so gui ships a local `modal()`), icon sidebar, toolbar
  (scan/add/search/view toggle), toast overlay, status bar, About modal.
- **`internal/ui` Session**: frontend-agnostic interactive core (state,
  commands, event stream, consent gating). The shirei GUI is a thin binding
  over it; a bubbletea TUI will bind to the same Session (decided, not yet
  built). One-shot CLI keeps using `internal/app` directly.

Still deferred in v0.2: the TUI itself, bulk install, edit mode, profiles
UI, GPU indicator, SteamGridDB key support, i18n, window-state persistence,
injection-method picker (dxgi only), native file dialogs inside shirei
(the OS picker is shelled out instead).

## Dependencies (settled)

- Vendored (`go mod vendor`, `vendor/` committed; `-mod=vendor` stays in CI and
  goreleaser).
- 7z: `github.com/bodgit/sevenzip` **gated by spike** against a real
  `Optiscaler_0.9.4-final*.7z` (BCJ2 risk); fallback = shell out to system `7z`.
- VDF: `github.com/lewisgibson/go-vdf`. No hand-rolled parser.
- GUI: `go.hasen.dev/shirei` **pinned v0.5.2**; all imports quarantined under
  `internal/gui`; upgrades are deliberate tasks.
- GitHub API: 15-minute cooldown + cached releases; fallback needs an explicit
  user prompt; requested vs resolved (asset, digest) recorded separately.
