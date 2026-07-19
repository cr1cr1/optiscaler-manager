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
  archive/    7z backend interface + sevenzip impl (or 7z shell-out)
  installer/  transaction core: stage → validate → backup → copy → manifest;
              rollback; uninstall; EAC check
  profile/    curated OptiScaler.ini writer
  gui/        ALL shirei imports; Action List view-model + views
```

`internal/installer` is the deep module: GUI and CLI both call it directly.
No service/orchestration layer between them (ceremony).

## Data flow

```
scan/install (CLI) ─┐
                    ├─→ discovery → classify → gh → archive → installer → store
GUI (frame gor.) ───┘        (domain packages never import shirei)
```

- Background goroutines do IO (scan, download, install); UI runs only on the
  shirei frame goroutine. State crosses via `WithFrameLock` +
  `RequestNextFrame`; commands via channels; cancellation via `context` at
  transaction boundaries only.
- View tests assert view-model state, never pixels (`RenderToPNG` smoke only).

## External state root

Manifests and backups live outside game directories under the platform data
dir (XDG: `$XDG_DATA_HOME/optiscaler-manager` or `~/.local/share/...`),
overridable for tests. Game directories are never used as our database.

## Why bundle-only

OptiScaler ≥ 0.9 bundles every satellite component into one archive. Treating
"OptiScaler" as one artifact removes the C# client's hardest problems
(version matrices, per-component caches, bundled-vs-separate toggles) and
collapses its six download pipelines into one.
