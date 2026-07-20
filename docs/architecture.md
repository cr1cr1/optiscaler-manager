---
type: explanation
---

# Architecture

## Shape

CLI-first Go app with a GUI default command. The existing kong shell
(`cmd/root.go`: `RootFlags`, `DefaultEnvars("OM")`, `ExitError` codes —
2 usage / 1 runtime, zerolog setup) is preserved; the GUI wraps it, never
replaces it. No-args launches the GUI via kong `default:"withargs"`.

## Package map

```
main.go                     thin wrapper → cmd.Run
docs_okf_test.go            OKF compliance gate (package main)
cmd/
  root.go version.go        existing shell (RootFlags; version subcommand)
  gui.go                    GuiCmd `cmd:"" default:"withargs"`; --audit-grid
  tui.go                    TuiCmd; second frontend on the same session
  session.go                newSession: shared session construction (gui + tui)
  scan.go                   headless game listing (store + versions columns)
  install.go uninstall.go   headless install/uninstall <path>
internal/
  domain/     Game (Store enum, AppName, ExePath, CompatPrefix), Release,
              Component, Kind, InstallStatus, Manifest, entries; Status
              state machine: 4 persisted (in_progress/committed/failed/
              rolled_back) + 1 derived (external, scan-time only)
  store/      manifest + backup persistence (external root)
  discovery/  multi-store scan. OS-agnostic parsers (Steam VDF, Epic .item,
              GOG goggame info, recursive roots, plist) test on every GOOS;
              build-tagged OS probes (Steam roots, Epic manifest dirs, GOG
              registry via a registry-reader seam, macOS /Applications .app,
              linux Proton compat prefix) compile per-GOOS. ScanAll merges
              Steam → Epic → GOG → apps → manual, deduped by canonical
              InstallDir; install-dir resolution
  classify/   upscaler kind+DLL detection (Dir, DirFiles)
  pever/      hostile-input PE version-resource parser (no cgo): FileVersion,
              MarketingName (vendored DLSS/FSR/XeSS version→name maps),
              OptiScalerVersion (manifest → log → ini evidence chain),
              DetectOptiScaler (external-install probe: injection-name
              candidates matched by PE version-info identity, bounded reads)
  gh/         GitHub releases: glob asset match, cooldown cache
  archive/    7z extraction with hostile-input defenses (sevenzip)
  installer/  transaction core: stage → validate → backup → copy → manifest;
              rollback; uninstall; EAC check; ctx cancel at phase boundaries
              (cleanup under context.WithoutCancel)
  profile/    curated OptiScaler.ini writer
  covers/     cover art: Steam CDN → store search → placeholder (disk cache)
  steam/      title → appid lookup (steamcommunity.com SearchApps; 30d TTL
              disk cache, no auth)
  protondb/   appid → compatibility tier (protondb.com summaries API; 7d
              TTL disk cache, 429 cooldown)
  settings/   persisted preferences (settings.json in the data root)
  pickdir/    OS directory dialog (zenity → kdialog)
  launch/     per-store per-OS command table (pure Command fn) + detached
              spawn (build-tagged spawners; Start + Process.Release, never
              Wait). Steam steam://rungameid (Proton is Steam's business),
              Epic launcher URL, GOG direct exe, manual user template split
              without a shell. Never `proton run`
  app/        shared orchestration: ScanLibrary, ScanAllLibraries (version
              enrichment via classify+pever on managed installs), Install,
              Uninstall, Rollback, ManualEntry, versioned bundle cache,
              ops.go (Op, RunOps: errgroup, first error cancels siblings)
  ui/         frontend-agnostic Session: state, commands, events, consent,
              per-game CancelOp, sort mode; cache.go is the games.json
              library cache (schema-versioned, atomic) behind Session.Start
  gui/        shirei binding over ui.Session (ALL shirei imports live here);
              theme tokens, arrow-key grid nav, right-docked detail panel
  tui/        bubbletea binding over ui.Session (renders snapshots, forwards
              keys; no business logic); multi-screen styled layout on
              bubbles spinner/textinput/viewport + lipgloss
```

`internal/installer` is the deep module for file transactions. `internal/app`
sequences domain packages into workflows both frontends share. `internal/ui`
adds interactive session semantics (async commands, event stream, consent
gates, toasts) with zero display-toolkit imports; `internal/gui` and
`internal/tui` render its snapshot and forward commands.

## Cross-platform shape (v0.3)

The tree cross-compiles to windows with CGO off (`GOOS=windows go vet ./...`
is a CI gate). Darwin is package-scoped only: shirei's cocoa backend needs an
Apple SDK, so CI gates darwin with vet + `go test -c` on the non-GUI packages
(`internal/discovery`, `internal/launch`). Cross-compiled test binaries are
never executed (no wine/darling on runners); the gate is compile-only (W3
decision). Release artifacts: linux/amd64, windows/amd64 (goreleaser, CGO
off). macOS artifacts wait on a macos-latest release runner — see
`.goreleaser.yml` for the full diagnosis.

## Cancellation model (v0.3)

Install/uninstall/rollback carry a context; checks sit at every phase
boundary. A cancelled op marks the manifest `failed` (cause recorded) and
rolls back under `context.WithoutCancel` — cleanup belongs to the same atomic
op, so it must outlive the dead op context while keeping the caller's values.
`internal/app/ops.go` runs batches as an errgroup: first error cancels
siblings. `ui.Session.CancelOp(gameDir)` cancels a single in-flight op and
settles the row to its pre-op status with one "Cancelled" event. Invariant #6
in `docs/safety.md`.

## Startup flow: games cache (v0.4)

Both frontends boot through `Session.Start(ctx)`. It reads `games.json`
from the data root (`internal/ui/cache.go`: schema-versioned envelope,
atomic temp-write + rename, best-effort writes that log and never fail the
caller). A warm cache hydrates the rows synchronously; each row's status is
then reconciled from the store's manifests (keyed by canonical install dir,
falling back to game root) so installs that settled while the manager was
not running show their real state. No PE parsing, no reclassification, no
scan; the status line reads `N games (cached)`. A missing, unreadable,
corrupt, stale-schema, or empty cache falls through to `Scan`. The cache is
rewritten after every scan, `AddDirectory`/`RemoveDirectory`, and op settle
(status change), serialized so concurrent writers cannot interleave.
Explicit rescans stay user-initiated (GUI Scan button, TUI `R`).

## External install detection (v0.6)

The enrich phase derives one more status. A game with no store manifest may
still carry an OptiScaler dropped in by hand; `app.ScanAllLibraries` probes
such unmanaged rows with `pever.DetectOptiScaler(injectionDir)`. The probe
stats the injection-name candidates (dxgi.dll, OptiScaler.dll, winmm.dll,
dbghelp.dll, version.dll, wininet.dll, winhttp.dll, d3d12.dll) and accepts a
candidate only when its PE StringFileInfo (ProductName, CompanyName, or
OriginalFilename) contains "optiscaler" — identity by version info, not
filename, so renamed shims count and DXVK's dxgi.dll does not. Reads are
bounded (size cap + LimitReader, same hardening as the rest of pever), the
probe runs inside the scan goroutine, and manifests stay authoritative:
managed games are never probed. A match yields the derived status
`domain.StatusExternal` with a version from the manifest.json →
OptiScaler.log → PE FileVersion chain; component versions are suppressed for
external rows (those DLLs are OptiScaler's, not the game's).

The status model is 4 persisted + 1 derived: `in_progress`, `committed`,
`failed`, `rolled_back` are written to store manifests; `external` exists
only at scan time and in the `games.json` cache (warm-cache reconcile keeps
external rows; manifests override only where they exist). Session semantics:
QuickInstall on an external row is an adopt — the installer backs the
external files up SHA-verified, so uninstall/rollback restores them
byte-identically and the post-uninstall re-detect (`pever.DetectOptiScaler`
on the row's injection dir) surfaces the row as external again. Uninstall of
a never-managed external row is refused up front with a clean toast (the
`app.ErrNotManaged` sentinel never leaks raw). `GameRow.CanOpenINI()`
(committed or external) gates Open INI in both frontends.

## Scan phases, progress, and online lookups (v0.5)

A scan runs as a pipeline of phases — discover → enrich → covers → lookup —
and reports `State.Progress{Phase, Done, Total}` as it goes (`EvScanProgress`
events); the GUI draws a progress bar under the toolbar, the TUI a progress
line. The lookup phase is online and optional: `internal/steam` resolves a
manual game's title to a Steam appid and `internal/protondb` resolves the
appid to a compatibility tier (Steam-library rows skip the search and query
the tier directly). It runs under a per-scan budget (8 rows), TTL disk
caches, and a 429 cooldown, degrades silently when offline, and is gated by
`online_lookups` (default true) — with either client nil or the setting off,
the phase is skipped entirely.

`AddDirectory` is asynchronous by design: the session validates the path,
persists settings, and inserts a placeholder row synchronously, then a
goroutine walks, classifies, covers, and online-enriches the directory and
replaces the placeholder. A duplicate add while one is in flight is
rejected. `ClearBundleCache` likewise runs off the frame goroutine.

## Data flow

```
scan/install (CLI) ─┐
                    ├─→ discovery → classify → gh → archive → installer → store
GUI/TUI (Session) ──┘        (domain packages never import shirei)
                 ↕ covers (Steam CDN / store search)

ui.Session: commands spawn goroutines; state mutated under its mutex;
frontends drain Events() and render Snapshot(). shirei rule: all UI calls on
the frame goroutine; the GUI binding drains events each frame (non-blocking)
and re-renders.
```

## External state root

Manifests and backups live outside game directories under the platform data
dir (XDG: `$XDG_DATA_HOME/optiscaler-manager` or `~/.local/share/...`),
overridable for tests. Game directories are never used as our database.

## Why bundle-only

OptiScaler ≥ 0.9 bundles every satellite component into one archive. Treating
"OptiScaler" as one artifact removes the C# client's hardest problems
(version matrices, per-component caches, bundled-vs-separate toggles) and
collapses its six download pipelines into one.
