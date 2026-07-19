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

## v0.3 scope (multi-store, versions, launch, TUI, cancel)

Added after v0.2 (waves W3–W5, release work W6). Decisions closed; reopen
only with new evidence.

- **Multi-store discovery**: Steam, Epic (.item manifests), GOG (Windows
  only — registry + `goggame-<id>.info`), macOS `/Applications` .app
  bundles, and manual recursive roots (settings ExtraDirs). `ScanAll` merges
  in that order, deduped by canonical install dir. `domain.Game` gains
  `Store` (enum; `StoreSteam` is the zero value), `AppName`, `ExePath`,
  `CompatPrefix`.
- **Windows + macOS scanning**: OS-agnostic parsers are tested on every GOOS;
  OS probes are build-tagged per platform (Steam roots incl. Windows
  registry, Epic manifest dirs, GOG Windows registry behind a reader seam,
  macOS plist parsing, linux Proton compat-prefix display).
- **Version display**: `internal/pever` parses PE version resources directly
  (no cgo, hostile-input safe) and maps raw versions to marketing names
  (DLSS/FSR/XeSS tables); OptiScaler version resolved via a manifest → log →
  ini evidence chain. Enrichment only on managed installs (committed manifest
  or OptiScaler.dll present) — no PE parsing for unmanaged games.
- **Game launching**: per-store per-OS command table in `internal/launch`.
  Steam: `steam steam://rungameid/<id>` (auto-Proton; Proton selection stays
  Steam's business — **never** `proton run`). Epic: launcher URL with
  AppName. GOG: direct exe (DRM-free). Manual: user template with
  `{exe}`/`{dir}`/`{appid}`/`{args}` placeholders, split without a shell.
  Detached spawn (Start + Release, never Wait); URL openers get a 10s cap.
  Fire-and-forget: spawn success proves nothing about the game running.
- **TUI frontend**: `optiscaler-manager tui` (bubbletea) binds the same
  `ui.Session`; shared session construction in `cmd/session.go`.
- **Cancellable ops**: ctx checks at every installer phase boundary; cancel ⇒
  manifest `failed` + automatic rollback under `context.WithoutCancel`;
  per-game `Session.CancelOp`; batched ops via errgroup (first error cancels
  siblings).
- **GUI polish**: full keyboard nav (Tab/Shift-Tab focus cycle, Enter/Space
  activation, Esc closes modals), sidebar Exit (flushes settings), responsive
  grid (1–8 cols, card width capped), version badges on cards, per-card and
  dashboard Launch buttons, busy-state Cancel button.

### v0.3 known limits

- Cross-GOOS correctness is **compile-gated only**: CI vets windows for the
  whole tree and vets + test-compiles darwin/windows for
  `internal/discovery` + `internal/launch`. Cross test binaries cannot
  execute on Linux runners (no wine/darling), so behavior on Windows/macOS is
  verified by compile and by linux-executed parser tests, not by running the
  suite there (W3 decision).
- Release artifacts are linux/amd64 + windows/amd64. **No macOS builds**:
  shirei v0.5.2's cocoa backend is cgo + Apple frameworks, which a Linux
  runner cannot link (needs an Apple SDK). Unlock path is a macos-latest
  runner; diagnosis lives in `.goreleaser.yml`.
- Epic launch needs the AppName from the .item manifest; without it the exe
  fallback fires.
- Launch is fire-and-forget: no process tracking, no "game is running" state.

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
