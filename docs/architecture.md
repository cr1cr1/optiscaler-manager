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
              Component, Kind, InstallStatus, Manifest, entries
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
              OptiScalerVersion (manifest → log → ini evidence chain)
  gh/         GitHub releases: glob asset match, cooldown cache
  archive/    7z extraction with hostile-input defenses (sevenzip)
  installer/  transaction core: stage → validate → backup → copy → manifest;
              rollback; uninstall; EAC check; ctx cancel at phase boundaries
              (cleanup under context.WithoutCancel)
  profile/    curated OptiScaler.ini writer
  covers/     cover art: Steam CDN → store search → placeholder (disk cache)
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
              per-game CancelOp
  gui/        shirei binding over ui.Session (ALL shirei imports live here)
  tui/        bubbletea binding over ui.Session (renders snapshots, forwards
              keys; no business logic)
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
