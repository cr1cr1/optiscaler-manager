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
- Release asset resolved by **glob**, never exact filename (names embed a
  date and a `_MM` marker). The glob is per distribution fork: upstream
  uses `Optiscaler_*.7z`; forks define their own (see the fork section
  below).
- After extraction, the bundle is validated against the one universal
  requirement — the injector dll (`OptiScaler.dll`, renamed to `dxgi.dll`
  on install); anything else is the distribution's own business. Each
  distribution's **archive listing is its install set** (v0.16): forks lay
  files out differently (DLSSNR ships no fakenvapi and keeps support DLLs
  under an `OptiScaler/` subdir), and members install verbatim, nested
  paths preserved, never stripped. Clutter is filtered (issue 026):
  markdown documentation and install/remove scripts
  (`.bat`/`.cmd`/`.ps1`/`.sh`) never reach the game dir, and no empty
  directories are created.
- The separate upstream downloads the reference C# client uses are stale:
  Nukem9/dlssg-to-fsr3 ≥ 0.130 has no GitHub assets (moved to Nexus Mods);
  OptiPatcher is a raw `.asi` and out of scope.

## Distribution forks

- The OptiScaler source is **selectable** (Settings, GUI and TUI): a list
  of forks — GitHub `owner/repo` slug + release-asset glob — with one
  active entry driving every install, switch, and "latest" resolution.
  Built-ins: upstream `optiscaler/OptiScaler` (`Optiscaler_*.7z`,
  undeletable) and `jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass`
  (`OptiScaler-NR-*.zip`); users add/delete their own.
- The choice is **global**, never per-game. Each install records the fork
  slug in its manifest; legacy manifests read as upstream.
- Everything on disk namespaces per fork
  (`<cache>/optiscaler/<fork-key>/`: release cache, cooldown, bundle
  dirs), so same-named tags from different distributions can never
  collide; the pre-fork flat cache migrates into the upstream namespace
  on startup.
- `.zip` bundles extract through the same sanitized pipeline as `.7z`
  (archive format dispatches on the matched asset's extension).
- **Fork switches preserve the old distribution** (v0.16): when a
  per-game version switch crosses forks (the manifest's fork ≠ the
  active source), the old distribution's file set — exactly what its
  archive listed, via the manifest — moves into
  `<injection-dir>/<repo-name>.YYMMDD/` (old fork's repo segment, local
  date; `-2`, `-3`… on same-day repeats) instead of being deleted.
  Overwritten files still restore their SHA-verified pre-install
  originals; foreign-modified files are still refused, never moved.
  Same-fork release switches keep the internal rollback backup only — no
  dated dirs pile up on version bumps. The user's `OptiScaler.ini` stays
  with the new install (the usual ini preservation), not the dated dir.

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
- **Originals are always backed up in the game directory first** (issue
  023): overwritten pre-existing files copy to
  `<installDir>/optiscaler-backups/files/` (SHA-verified, restored on
  uninstall/rollback, removed when the manifest settles) — never to the
  app's state root. Any pending backup over 100 MiB (total per operation)
  pauses for explicit consent BEFORE a byte is touched; declining — or a
  non-interactive CLI — aborts the operation untouched. Pre-023 central
  backups are not read (clean break): uninstalling an install committed
  before this change errors on the missing backup instead of silently
  losing the originals.

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
- **Covers**: Steam CDN `library_600x900.jpg` by appid (primary) →
  SteamGridDB grid (when `steamgriddb_key` is set in settings.json) →
  PCGamingWiki box art (keyless: opensearch + wikitext infobox) → Steam
  store search (name→appid, zero-key fallback) → generated placeholder.
  Cached on disk by sanitized appid; every cached image is normalized to
  the 2:3 card aspect (center-crop — landscape hero banners included,
  legacy files scrubbed on read, issue 025). (Ecosystem-verified keyless
  pattern: Lutris and Heroic use the same Steam CDN primary; Bottles uses
  a private SteamGridDB proxy, not copyable — we call SteamGridDB
  directly with the user's own free API key, issue 024.)
- **Bundle cache**: OptiScaler bundles at
  `$XDG_CACHE_HOME/optiscaler-manager/optiscaler/<version>/` (default
  `~/.cache/...`), reused before any download (`OM_CACHE_DIR` overrides).
- **Settings** (persisted `settings.json` in the data root): default
  OptiScaler version (tag or `latest`), manually added game directories;
  settings window with version input and clear-cache action.
- **Manual game add**: OS directory dialog (Linux: zenity → kdialog;
  Windows: PowerShell FolderBrowserDialog; macOS: osascript) via
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
  (no cgo, hostile-input safe). Pill labels: DLSS shows the raw DLL version
  in tag form (`DLSS 310.5.3` — marketing names cannot reflect a version
  switch, issue 019); FSR/XeSS map raw versions to marketing names.
  OptiScaler version resolved via a manifest → log →
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
  shirei v0.6.7's cocoa backend is cgo + Apple frameworks, which a Linux
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
  scan. (Partially superseded in v0.13: selecting a row re-probes install
  state and external-row versions from disk; covers and committed-row
  versions still wait for a rescan.)
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
  (discover/covers/lookup) with Done/Total; the GUI renders a
  progress bar under the toolbar, the TUI a progress line with phase, bar,
  and percent. Rows stream into the grid as they are discovered — existing
  cards refresh in place, new cards append, cover art pops in as it
  resolves — and the settle re-sorts and prunes once at the end.
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
  Component versions parse for external rows too (revised v0.14f): those
  DLLs — NVIDIA runtime, FSR, XeSS — belong to the game; OptiScaler's
  bundle ships none of them, and suppressing the versions hid the DLSS
  update pill from hand-installed games.
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
  The detail panel also opens the game's binary dir in the OS file
  manager (`Session.OpenGameFolder`, issue 027: `xdg-open` /
  Finder / Explorer; not install-gated).

### v0.6 known limits

- Detection requires a PE-branded injection DLL in the injection dir: stale
  `OptiScaler.ini`/`OptiScaler.log` remnants alone do not count as an
  external install.
- The external status is derived at scan time and cached in `games.json`
  until the next rescan; external installs added or removed while the
  manager is not running surface only after a rescan. (Superseded in
  v0.13: the warm-boot reconcile and the selection-time re-probe surface
  hand add/remove/toggle without a rescan — see the v0.13 section.)
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
  the scan, 6 in classification), and the main-exe search descends at
  most 4 levels (deep enough for Prey's
  `Binaries/Danielle/x64-Epic/Release` layout; anything deeper is
  invisible). Exe walks never descend pure-plumbing subtrees
  (`downloading`, `compatdata`, `shadercache`, `temp`, `music`,
  `sourcemods`, `steamworks*`, `steamvr`, `workshop`) nor a Wine prefix's
  `drive_c/windows` or `drive_c/users`, and never descends Proton /
  SteamLinuxRuntime / `ThirdParty` tooling trees — a game whose only exe
  lived under one of those would be missed (none known; Lutris games
  under `drive_c/Program Files` / `GOG Games` are unaffected).
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
- GUI: `go.hasen.dev/shirei` **pinned v0.6.7**; all imports quarantined under
  `internal/gui`; upgrades are deliberate tasks.
- GitHub API: 15-minute cooldown + cached releases; fallback needs an explicit
  user prompt; requested vs resolved (asset, digest) recorded separately.

### v0.8 identification (rules and limits)

- Manual rows are identified by priority: `title_overrides` (settings,
  keyed by canonical install dir) > `steam_appid.txt` (root or ≤2
  levels, appid 480 and non-numeric rejected) > `goggame-*.info` >
  `.egstore` (InstallLocation must match the dir) > Unity
  `*_Data/app.info` > normalized fuzzy store match > PE metadata > exe
  stem > folder. Steam/Epic/GOG store-manifest rows keep their launcher
  names (untouched).
- The fuzzy match is deliberately strict: exact normalized equality or
  Jaccard ≥ 90 on token sets (no subset credit), PC platform bonus,
  edition-mismatch penalty; scores of 75–89 (in practice, 80) need the
  store developer to match the PE CompanyName. Titles of ≤3 normalized
  chars are never queried, and ≤4 only ever match exactly. When Steam
  has no match, PCGamingWiki is the secondary canonical source (keyless
  Cargo/opensearch, 30 req/min, cached; GOG/off-store games). False
  accepts are still possible for genuinely ambiguous names; the
  override table is the escape hatch. Known limits: engine-metadata
  codenames (e.g. a Unity product string like "STASIS2") stop the
  pipeline by design; DLC store pages resolve only when no base app
  matches; edition variants sharing one name (Crysis vs Crysis
  Remastered) resolve to the first store hit unless an appid file or
  override disambiguates.
- Normalization strips edition/repack noise for MATCHING only — the
  canonical store name becomes the display title, so a folder "Dead
  Space Remake" can legitimately become "Dead Space".
- Online resolution is keyless Steam only (appdetails, storesearch);
  IGDB/SteamGridDB need user credentials and are out of scope, and the
  formerly keyless GetAppList bulk dump no longer works without an API
  key, so there is no offline canonical corpus — offline scans keep the
  PE/stem/folder tail (sources recorded; the next online scan upgrades).
- Rows persist `TitleSource` + `SteamAppID` (games cache v5; v1–v4
  invalidated).

## Later shipped scope (v0.9–v0.13)

Shipped and recorded in `docs/log.md`; collected here because they settled
decisions the per-version sections above do not cover.

- **ProtonDB is Linux-only** (v0.9.0): enrichment skips the ProtonDB
  summary call off-Linux (injectable GOOS seam on `ui.Deps`), and
  `games.json` caches written on Linux are stripped of tiers when loaded
  off-Linux (strip-at-load, no schema bump). The resolved Steam appid is
  still kept for identification on every platform.
- **Per-game version switching** (v0.10): `Session.SwitchVersion` switches
  an installed game to a chosen OptiScaler version while preserving the
  game's `OptiScaler.ini` (captured before the switch, written back after
  the install leg, removed before the uninstall leg so foreign-modified
  files don't block it). Committed rows chain uninstall→install at the
  chosen tag; external rows adopt-install. The selectable list is
  unique(installed ∪ cached bundles ∪ preference), semver-descending. The
  old upgrade-offer model is retired in favor of this explicit management.
- **umu-launcher integration** (v0.12, Linux only, opt-in): when
  `UmuEnabled` is on and `umu-run` is on PATH, manual-store games whose
  ExePath is a Windows binary launch through umu instead of the direct
  exe path. Each game gets a deterministic Proton prefix at
  `<state-root>/umu-prefixes/<sha1(installDir)[:12]>`; the Proton build is
  user-pinnable via `UmuProtonPath` (empty = auto-detect from Steam
  `compatibilitytools.d`, Bottles `runners/`, and umu
  `compatibilitytools`).
- **Hook disable/enable toggle** (v0.13): installed games get a
  Disable/Enable affordance (GUI button in the detail panel, "disabled"
  pill on the card; TUI `d` on the detail screen) that renames the injection hook
  to a parked `<name>.disabled` (or restores it). The install keeps its
  status and files; the parked state renders as a "disabled" pill. The
  disable direction renames only an identity-verified OptiScaler hook
  (a DXVK dxgi.dll is refused); the enable direction accepts any parked
  variant of a known hook name, including hand backup-style suffixes
  (`.1`, `.bak`).
- **Selection-time install re-probe** (v0.13): selecting a game (GUI
  card/list click, TUI detail-open) re-probes the injection dir from
  disk, so hooks installed, removed, or renamed by hand since the last
  scan render correctly without a rescan. Committed rows keep manifest
  status; external rows follow the disk exactly; interrupted and
  rolled-back rows are skipped (the repair surface owns them).
- **Cover-art fallback for manual games** (v0.13): the recent CDN miss
  marker no longer suppresses the title search (it skips only the CDN
  retry); store-search binding prefers the best-scored acceptable
  candidate; `custom_<folder>` ids never reach the CDN appid path;
  identified rows retry the search with the resolved title.
- **Card size setting** (v0.13): `settings.CardSize`
  (small/medium/large, `OrDefault()` fallback) drives the GUI grid
  card-size preset; GUI Settings selector + `Session.SetCardSize`.
- **Interrupted-install repair surface at boot** (v0.13): warm boot
  toasts installs left in_progress/failed; the GUI shows a persistent
  banner and the TUI a warning line, both derived from row state so
  cold and warm boots are covered; plain CLI commands print a stderr
  warning naming the manifest (gated off for gui/tui/version).
- **Refactors** (v0.13): `internal/jsoncache` shared JSON cache/cooldown
  helpers (steam, protondb, storesearch caches); `settings.CardSize`
  type; `Session.updateSettings` helper for the persisted setters;
  `internal/ui` split into themed files; shared row helpers
  (`HasInstall`, `DisableToggleLabel`, `InterruptedRows`); Go 1.27 +
  shirei v0.6.7 with the vendor patches reapplied.

## v0.14 scope (NVIDIA DLSS runtime updater)

- **On-demand installs, startup pre-warm**: the DLSS version pill (GUI
  card and detail panel) doubles as the update control; nothing is
  INSTALLED into a game without a press, and no NVIDIA bytes ship with
  this app. The startup check pre-downloads the latest published set
  into the download cache (v0.15), so a press is cache-first from the
  first moment.
- **Three-file transaction**: `nvngx_dlss.dll`, `nvngx_dlssd.dll`,
  `nvngx_dlssg.dll` update together; the complete existing set is
  required (no injection of components a game never shipped).
- **Immutable-commit source**: NVIDIA/DLSS `main` resolves to a commit
  SHA via the GitHub API; files come from
  `lib/Windows_x86_64/rel/<name>` at that exact commit. No mutable-URL
  fetches, no release/version picker. The published version is learned
  from ONE tags-API call on startup (greatest version among the fetched
  tags → version + commit) and that latest set is pre-downloaded into
  the cache at startup (v0.15, zero network when already cached); a
  press is cache-first: the
  known commit — the published tag's, or the download cache's newest when
  the online half is unknown — is used only while it is complete in the
  download cache, else `main` re-resolves. Warm-boot caches from older
  app versions are invalidated (schema 6) so the new row fields are
  always present.
- **Snapshot backups + restore menu**: every update and restore first
  persists a hash-verified snapshot of the current set inside the game
  directory itself (`dlss-backups/<timestamp>_dlss-<version>/`, issue
  022 — backups travel with the game folder and can be recovered by
  hand); the pill's shared dropdown arrow lists
  them (newest first) through the same dropdown machinery as the version
  picker (v0.14i: the focused arrow trigger drives Down/Up/Enter
  highlight navigation, hover adopts on motion, Esc/click-outside
  dismiss), a pick asks for confirmation, and a confirmed restore swaps
  the complete set back after verifying snapshot hashes. Backups
  deduplicate by member digests (identical sets reuse the existing
  snapshot — and a dedup hit plans zero new bytes, so it never trips the
  100 MiB consent gate, issue 023), and a press on an already-current
  set is a graceful no-op
  ("already at <v>") with no snapshot and no file changes.
- **Commit-keyed download cache**: same layout as the OptiScaler bundle
  cache — downloaded NVIDIA DLLs persist per source commit under
  `<cacheDir>/dlss/<commit>/` — plus a `manifest.json` pinning each
  member's SHA-256. Cached members are re-verified on every update,
  freshly downloaded members are PE-validated before their hash is
  recorded (a lying 200 never earns a manifest entry), and a moved
  `main` resolves to a fresh commit dir whose bytes are fetched; a
  cached update costs one commits-API call and no DLL downloads.
- **Session integration**: the ops ride the per-game busy/cancel slot;
  success re-probes component versions so the pill updates immediately.
- **Plain-game component enrichment**: component versions now parse for
  every row with a resolved injection dir (external rows included since
  v0.14f), so a game with DLSS but no OptiScaler install still shows
  the pressable DLSS label. A DLL whose version resource does not
  parse degrades to a bare `DLSS` pill that stays pressable.
  `classify.Dir` already walked those
  dirs for tech badges; only detected component DLLs get a bounded PE
  read.
- **Deferred**: CLI surfaces, DLSS-FG/DLSSD-only actions, version
  picker, scheduled checks, bulk update,
  snapshot pruning (the newest-first menu plus ~115 MB per snapshot
  stays acceptable; revisit when a user accumulates dozens).

## v0.15 scope (startup latest pre-warm + named Latest option)

- **Startup checks both runtimes and pre-downloads `latest`** (user
  spec: "at program startup, optiscaler and nvidia DLSS dlls versions
  are checked, and `latest` downloaded in the cache directory"):
  `Session.Start` now runs `startupPreload` — resolve OptiScaler
  `latest` once, memoize its tag, download its bundle into
  `<cacheDir>/optiscaler/<tag>/` when not already cached — alongside
  `CheckDLSS`, whose tags goroutine now ALSO pre-downloads the latest
  published three-DLL set via `dlss.Client.Preload` (one tags call,
  zero network when the commit is already cached). Both are async and
  failure-silent; an offline boot just leaves the concrete cached
  versions and the next press resolves online as before.
- **Named `Latest (tag)` option in the version dropdown**: the memoized
  startup tag is rendered as a first-class row — it ABSORBS the
  concrete entry when that entry IS the latest (one row, one tick, so
  a latest-installed game shows a single `Latest (…)` row that reads
  as latest, not an indistinguishable cached version) or PREPENDS when
  absent (latest is the maximum, so it sorts first). Picking it
  dispatches the literal `latest`; `SwitchVersion` re-resolves it to a
  concrete tag at pick time (fresh resolve, not the possibly-stale
  memo) before the chain starts, so the same-version no-op guard, the
  EAC consent pin, and the install leg all see a real tag. Offline
  boot (no known latest) renders the plain concrete list.
- **Concurrency + freshness hardening**: `gh.Client` is mutex-guarded
  (a startup scan and the preload can `Resolve` at the same instant);
  a release cache written by a LIVE fetch in this process is
  provably fresh (`gh.CacheFresh`), so an install served from it skips
  the stale-cache consent prompt — whose "stale" premise does not hold
  for data fetched moments earlier. The stale-cache gate still asks for
  a cache a PREVIOUS process left behind.

## v0.15 deferred

(v0.16 pulled the TUI `Latest` option in; nothing remains deferred from
v0.15.)

## v0.16 scope (answered-only cooldown; TUI Latest option)

- **gh cooldown starts only on an answered API call** (H4): a transport
  failure, a non-200 response, or a decode failure no longer writes
  `cooldown.json`, so a transient network blip no longer locks every
  resolve out of the network for 15 minutes behind a misleading
  `ErrRateLimited`. A rate-limited response still starts the back-off
  window (serve the possibly-stale cache meanwhile), and a successful
  fetch still starts it (later resolves — this or another process —
  serve the just-written cache); the success write happens after
  `releases.json` so a crash can never leave cooldown-without-cache.
- **TUI `Latest` option** (the v0.15 deferral, pulled in): when the
  startup memo knows the latest tag, the `v` version cycle replaces the
  list entry semver-equal to it with a single `Latest (tag)` row — in
  place, prepended when absent (GUI parity) — and Enter dispatches the
  literal `latest`, which the session core re-resolves at pick time.
  Confirming an absorbed row — the current version — stays the S13
  no-op; without a known latest the cycle is unchanged. While staging,
  the games cell shows a short `→ Latest` (a mid-tag cut like
  `→ Latest (v0.9…` would read as version v0.9); the detail line renders
  the full label.

## v0.16 scope (CLI surfaces)

The post-v0.1 features existed only behind the GUI/TUI; v0.16 exposes
them as one-shot commands over the same session core:

- `switch <dir> [--version <tag|latest>]` — switch a game's OptiScaler
  version; an empty `--version` means the configured default, `latest`
  re-resolves at pick time (the v0.15 seam), and switching to the
  installed version reports "already at" without running an op.
- `dlss-update <dir>` — update the game's NVIDIA DLSS runtime set
  (cache-first; a partial or absent current set warns and proceeds
  without a rollback backup, installing the missing members too).
- `dlss-restore <dir> [--snapshot <id>]` — restore a backed-up set;
  the empty id means the newest snapshot. Runs behind the restore
  consent gate.
- `launch <dir>` — fire-and-forget launch request (reports "requested",
  never "launched").
- `hook <dir> --enable|--disable` — park/un-park the installed hook
  (exactly one state required; the rename is atomic and synchronous).

Behavior: each command boots the shared `ui.Session`, waits for the
games list to settle, dispatches, and blocks on a CLI event waiter
until the op's terminal event (`EvOpSettled` for the compound version
switch, done/failed/cancelled for single-leg ops). Consent gates are
answered on the terminal (y/n); a non-interactive stdin declines and
the command exits 1 — consent is never bypassed and there is no
`--yes` flag. `--timeout` (default 10m) bounds every wait (the
synchronous `hook` toggle has none). Exit codes: 0 success, 1 runtime
failure, 2 usage.

Explicitly settled in v0.16: version tracking stays stable-only (the
OptiScaler-nightly repository's prerelease builds are not tracked), and
settings commands / a `--json` scripting mode stay deferred.
