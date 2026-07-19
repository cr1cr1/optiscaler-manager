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
  scan.go                   headless game listing
  install.go uninstall.go   headless install/uninstall <path>
internal/
  domain/     Game, Release, Component, Kind, InstallStatus, Manifest, entries
  store/      manifest + backup persistence (external root)
  discovery/  Steam library scan (go-vdf), install-dir resolution
  classify/   upscaler kind+DLL detection
  gh/         GitHub releases: glob asset match, cooldown cache
  archive/    7z extraction with hostile-input defenses (sevenzip)
  installer/  transaction core: stage → validate → backup → copy → manifest;
              rollback; uninstall; EAC check
  profile/    curated OptiScaler.ini writer
  covers/     cover art: Steam CDN → store search → placeholder (disk cache)
  settings/   persisted preferences (settings.json in the data root)
  pickdir/    OS directory dialog (zenity → kdialog)
  app/        shared orchestration: ScanLibrary, Install, Uninstall, Rollback,
              ManualEntry, versioned bundle cache
  ui/         frontend-agnostic Session: state, commands, events, consent
  gui/        shirei binding over ui.Session (ALL shirei imports live here)
```

`internal/installer` is the deep module for file transactions. `internal/app`
sequences domain packages into workflows both frontends share. `internal/ui`
adds interactive session semantics (async commands, event stream, consent
gates, toasts) with zero display-toolkit imports; `internal/gui` renders its
snapshot and forwards commands. A future TUI binds to the same Session.

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
