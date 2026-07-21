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

## v0.4 scope (settings UI, games cache, GUI polish, TUI overhaul)

Delivered 2026-07-20 (waves W1–W2). Decisions closed; reopen only with new
evidence.

- **Scan directories managed in-app**: Settings (GUI modal "Scan
  Directories" section, TUI screen `2`) lists `ExtraDirs` with add and
  per-row remove. GUI: "Add directory…" opens the OS picker, each row has a
  Remove button. TUI: `a` adds a path inline, `d` removes the selected row
  behind a `y`/`n` confirm. New session commands: `RemoveDirectory` (drops
  the directory's row and any nested games scanned under it, persists
  settings, rewrites the games cache; unknown dirs are a silent no-op) and
  `SetSort` (`SortDefault` actionable-first / `SortName` A–Z).
- **Launch template edited in-app**: GUI Settings "Launch Template" field
  and TUI `t`; both persist through `Session.SetLaunchTemplate`. Editing
  `settings.json` by hand is no longer required (the file format is
  unchanged).
- **Games cache (cache-first startup)**: `games.json` in the data root,
  schema-versioned (`version: 1`), written atomically (temp + rename).
  `Session.Start` boots cache-first: a warm cache hydrates rows
  synchronously with each row's status reconciled from store manifests (no
  PE parsing, no reclassification, no scan) and reports
  `N games (cached)`; a missing, unreadable, corrupt, stale-schema, or
  empty cache falls through to a full scan. The cache is rewritten
  (best-effort, serialized, never fails the caller) after scans,
  Add/RemoveDirectory, and op settles. Explicit rescan stays
  user-initiated: GUI Scan button, TUI `R`.
- **GUI polish**: theme tokens (spacing, radii, elevation, expanded
  palette), card/row hover states, deterministic gradient cover
  placeholders (glyph + title initial, FNV-hashed hue) replacing the tiny
  dark no-art tile, right-docked detail side panel replacing the dashboard
  modal (grid stays visible beside it), toolbar sort menu (Default / Name)
  + icon grid/list switch + scan spinner, empty states with icon, heading,
  and CTA buttons (scan/add-directory, clear-search), 72px icon sidebar
  with active-section accent, arrow-key grid navigation (±1 across, ±cols
  up/down; Enter opens the detail panel, Esc closes it) with status-bar
  shortcut hints, raised toast cards with tone accent bar capped at three,
  themed scrollbars and dark search input, and sort/view/search controls
  disabled while the library is empty.
- **TUI overhaul**: number-key screens (1 Games / 2 Settings / 3 Help),
  styled game columns (badges, title, store, version, status with tone
  colors), detail screen (`enter`) with actions `i`/`l`/`c`/`r`/`o` and
  `esc` back, live `/` filter (Esc clears), `s` sort toggle, `R` rescan,
  `i` quick install, `q` + `ctrl+c` quit, centered confirm modal
  (`[y]` proceed / `[n]` cancel), busy spinner, toasts, resize-aware
  layout, and empty-state guidance. Deps: bubbles v1.0.0 (vendored),
  lipgloss promoted to a direct dependency.

### v0.4 known limits

- The GUI search field is click-to-focus only; there is no `/` focus
  shortcut in the GUI (that keybind is TUI-only).
- Cache hydration reconciles install status from manifests only; version
  strings and covers shown at boot are the cached values until the next
  scan.
- macOS remains blocked exactly as in v0.3 (shirei cocoa backend needs an
  Apple SDK); no macOS support is claimed.

## v0.5 scope (PE titles, ProtonDB tiers, progress, async ops, TUI fixes)

Delivered 2026-07-20 (waves W1–W3). Decisions closed; reopen only with new
evidence.

- **PE game titles**: manual/recursive games get their title from PE
  version info (`ProductName` → `FileDescription` → folder-name fallback),
  so Windows exes get real titles even when scanning on Linux. Linux
  recursive scans also accept `.exe` files without the execute bit
  (previously-missed games now appear).
- **Online lookups**: a new scan phase resolves manual games title → Steam
  appid (`steamcommunity.com/actions/SearchApps`, new `internal/steam`
  client) → ProtonDB tier (`protondb.com` summaries API, new
  `internal/protondb` client); numeric-appid (Steam-library) rows get the
  tier directly. Per-scan budget of 8 lookups, TTL disk caches (30 days
  search / 7 days summaries), 429 cooldown, silent offline degradation.
  Gated by `online_lookups` in settings.json (default **true**; GUI
  Settings toggle "Online game info (Steam/ProtonDB)", TUI settings `o`).
- **Scan progress**: `State.Progress` reports the phase
  (discover/enrich/covers/lookup) with Done/Total; the GUI renders a
  progress bar under the toolbar, the TUI a progress line with phase, bar,
  and percent.
- **Async ops**: `AddDirectory` and `ClearBundleCache` are non-blocking.
  AddDirectory shows a placeholder row instantly, then enriches it in a
  goroutine; a duplicate add while one is in flight is rejected.
- **GUI fixes**: sidebar nav items are uniform width (Expand); card buttons
  fire their action without opening the detail panel — only card-body
  clicks open details; the detail panel is proportional (30% of the
  window, clamped 300–480px); ProtonDB tier pills (platinum/gold/silver/
  bronze/borked/pending) on cards and the detail panel; dark Wayland CSD
  titlebar via a vendor patch (`docs/vendor-patches.md`).
- **TUI fixes**: the tab bar renders again — the root cause of "no access
  to Settings" was View emitting h+1 lines, which made bubbletea's
  renderer drop line 0; View now emits exactly h lines. Every screen
  footer shows screen-switch hints (1 games · 2 settings · 3 help ·
  4 about); new About screen (key 4) with the build version plumbed from
  cmd and the stack line; escape hints in input modes and confirm modals;
  ProtonDB tier in the games table badges and detail.

### v0.5 known limits

- The dark CSD titlebar is **Wayland-only**; on X11 the window manager
  draws the decorations.
- ProtonDB tiers shown between scans are cached values; they refresh on
  the next scan, subject to the disk-cache TTLs.
- With online lookups enabled, game titles are sent to steamcommunity.com
  and appids to protondb.com. Disable the toggle (or `online_lookups`) for
  a fully offline scan.
- The vendor patch does not survive `go mod vendor`; it must be reapplied
  (the `TestVendorCSDPatchPresent` guard fails loudly when it is missing).

## v0.6 scope (external OptiScaler detection + adopt)

Delivered 2026-07-20 (tasks T1–T7; T8 docs, T9 review gate). Decisions
closed; reopen only with new evidence.

- **External detection**: games scanned without a manager manifest are
  probed for a pre-existing OptiScaler install. `pever.DetectOptiScaler`
  checks the injection-name candidates (dxgi.dll, OptiScaler.dll, winmm.dll,
  dbghelp.dll, version.dll, wininet.dll, winhttp.dll, d3d12.dll) and
  identifies OptiScaler by PE version-info identity — ProductName,
  CompanyName, or OriginalFilename containing "optiscaler"
  (case-insensitive). OriginalFilename survives renames, so a shim renamed
  to dxgi.dll is still recognized and a DXVK dxgi.dll is not a false
  positive. Version evidence chain: OptiScaler's own `manifest.json` →
  `OptiScaler.log` banner → the matched DLL's PE FileVersion.
- **Bounded, unmanaged-only, async**: the probe runs inside the scan
  goroutine (no blocking on UI paths) with bounded reads, and only on
  unmanaged games — a store manifest stays authoritative where one exists.
  Component versions are suppressed for external rows: those DLLs belong to
  OptiScaler's bundle, not the game.
- **Derived status `external`**: `domain.StatusExternal` is computed at scan
  time and NEVER persisted to store manifests — the persisted state machine
  stays the four statuses. It renders in the GUI ("external", blue pill),
  the TUI (accent), and CLI scan output (`[external]`); the `games.json`
  cache carries it until the next rescan (warm-cache reconcile keeps
  external rows).
- **Adopt / refuse / restore**: QuickInstall on an external row reads
  "Adopt" — installing over the external files backs them up SHA-verified
  and makes the game managed. Uninstall or rollback then RESTORES the
  external files (keystone-tested: byte-identical restore, status returns to
  external). Uninstall of a never-managed external install is refused with a
  clean toast ("not installed by this manager — adopt first or remove
  manually"); no op is registered, no raw store sentinel leaks. After a
  managed uninstall, detection re-runs so a restored external install shows
  correctly. Open INI works on external installs (`GameRow.CanOpenINI`).

### v0.6 known limits

- Detection requires a PE-branded injection DLL in the injection dir: stale
  `OptiScaler.ini`/`OptiScaler.log` remnants alone do not count as an
  external install.
- The external status is derived at scan time and cached in `games.json`
  until the next rescan; external installs added or removed while the
  manager is not running surface only after a rescan.
- Detection only runs for games with a resolvable injection dir.
- Component versions stay hidden for external rows (see above) even when the
  external bundle's component DLLs are present.

## v0.7 scope (game-dir vs container classification + session integration)

- **`discovery.ClassifyGameDir(ctx, dir) (GameDirKind, error)`** (T1): sorts
  a directory into `GameDirGame`, `GameDirContainer`, or `GameDirEmpty`
  using only stats and bounded walks (no PE parsing); candidacy, skip
  tokens, and the depth cap are exactly the recursive scanner's.
  `LooksLikeGameDir` is the boolean form. Rules: an exe at depth ≤ 1 →
  game; no gamey children → empty; exactly one gamey child with the exe
  within depth ≤ 2 (engine layouts like Binaries/Win64) → game; otherwise
  → container. The recursive scan skips exe-less subdirectories instead of
  surfacing phantom rows.
- **Session integration** (T2): scans gate extra-dir self-rows on the
  classification — container/empty roots get no `ManualEntry` row (their
  games surface via the recursive scan), cover-progress totals exclude
  them, and the in-flight merge no longer resurrects stale container rows
  from pre-gating `games.json` caches. Roots that fail classification keep
  the previous row-bearing behavior.
- **`AddDirectory` three-way branch** (T2): the picked directory is
  classified synchronously (bounded, cheap — an explicit user action). Game
  → the v0.5 async contract unchanged (placeholder row, background
  enrichment, "directory added" event). Container → registered as a scan
  root: settings persisted synchronously, no placeholder/self-row, a
  "registered `<base>` as a scan folder" toast, and a background rescan
  surfaces its games. Empty → refused with a "no games found under
  `<base>`" warning; settings untouched, no op slot held. Classification
  failure falls through to the game flow.
- **Title priority pins** (T2): named characterization tests lock the chain
  PE ProductName → FileDescription → exe stem → folder name for manual
  entries, including the AddDirectory placeholder (folder title) being
  replaced by the enriched row (PE title).

### v0.7 known limits

- A directory that is both a game and a container (its own exe at top
  level plus game subdirectories) yields its own row AND one row per
  contained game — but only when none of its children is a container: a
  container child outranks the own exe (a Steam client dir with
  `steam.exe` next to `SteamApps` is a scan root, never a game row).
- Engine-folder detection is name-based (`bin`, `Binaries`, `Win64`,
  `x64`, `engine`, `redist`, `bin64`, `retail`, `exe`, …) plus platform
  plumbing (`drive_c`, `compatdata`, `shadercache`, `downloading`,
  `temp`, `music`, `sourcemods`, `__installer`, `_redist`, `Steamworks
  Shared`, versioned `Proton*` / `SteamLinuxRuntime*` folders). Engine
  folders never row and never make their parent a container. A real game
  literally named like one would be skipped; an unusual engine layout
  not covered could still row. Container nesting is bounded (4 levels in
  the scan, 6 in classification).
- A `steam.exe` + `Steam.dll` pair marks a platform client install,
  which always classifies as a container. A game shipping both files at
  its root would be misclassified (not seen in practice).
- Exe candidacy on unix requires PE/ELF magic bytes, so extensionless
  scripts and data files with the execute bit are ignored; a game
  shipped as a raw script (`#!`) is not detected (acceptable: OptiScaler
  targets Windows binaries).
- Titles come from PE metadata first (windowed reads, no size cap),
  then the exe stem (platform tokens stripped), then the folder. Some
  vendors ship unhelpful metadata (codenames like `Cardinal`, `Anvil`,
  `b1`; repacked exes with junk strings) — the chain is deliberately
  metadata-first per the contract.
- Adding a Proton folder or `compatdata` tree *directly* as a scan
  directory is refused (engine-named roots hold no games of their own).
- Warm caches written before v0.7.2 (schemas v1–v3) are invalidated by
  the v4 schema: the first v0.7.2 boot falls through to a real scan
  instead of showing rows the new scanner rejects (platform dirs,
  steamapps plumbing, engine/redist folders, capped-reader titles).

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
