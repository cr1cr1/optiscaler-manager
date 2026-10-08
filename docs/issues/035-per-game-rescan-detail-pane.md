---
title: Per-game rescan from the detail pane
description: Rescan button on the selected game re-runs the full scan pipeline (discovery, covers, online identification) for that one game only, reusing the global scan's methods.
---

# 035 — Per-game rescan from the detail pane

## What and why

The toolbar Scan rescans the whole library. After title/cover heuristic
fixes (issues 030/033/034) the user must rescan all 100+ games — with the
multi-source variant walks that is slow — just to refresh one game's title
or poster. Add a "Rescan" button to the GUI detail pane that runs the same
pipeline stages for the selected game only.

The work reuses the existing methods rather than duplicating logic:

- **Discovery**: a new `app.ScanGame(ctx, store, dir, opts)` re-discovers
  and enriches one game by canonical install dir through the same
  source-specific paths `ScanAllLibraries` uses — Steam via
  `discovery.ScanAll` (per-root appmanifests + launcher sources, matched
  by dir), manual scan-root children via
  `discovery.ScanRecursiveWithResolver(dir)`, manual self-rows (dir is
  itself an extra dir) via `app.ManualEntryWithResolver` — mirroring the
  global scan's precedence exactly (store sources first, manual fallback).
- **Pipeline**: `Session.RescanGame(ctx, installDir)` runs the same stages
  as `runScan`/`AddDirectory`: `baseRow` → `resolveCover` → online
  `identifyRow`/`enrichRow`/`refreshCovers` (gated on the online-lookups
  setting) → upsert → `sortRows`/`disambiguateTitles` → `persistCache`.
- **Op discipline**: the rescan registers on the per-game op registry
  (`registerOp`), so the detail pane shows the existing "Working…"/Cancel
  affordance and installs/adds on the same game are serialized against it.

- **Frontends**: the GUI detail pane gets a "Rescan" button (refresh
  icon) in its action row; the TUI detail view gets a rescan keybinding
  in its action list. Both call the same `Session.RescanGame`.

A game that no longer resolves anywhere (deleted, uninstalled from its
store) leaves the row untouched and toasts an honest "no longer found";
pruning rows stays the global scan's job.

Non-goal: removing vanished games. Re-scanning while a global scan is in
flight is allowed (both write full rows, last wins, converges at the
global settle).

## Acceptance

- [x] `app.ScanGame` returns the enriched entry for a game of each source
  (steam manifest, extra-dir child, extra-dir self-row) using the same
  discovery functions as the global scan.
- [x] `Session.RescanGame` refreshes the selected row's title/appid/cover
  through the same fakes the identify tests use, persists the cache, and
  toasts; the row of a vanished game is left untouched with a toast.
- [x] The detail pane renders a "Rescan" button (refresh icon) in the
  action row for any selected game; activation calls
  `Session.RescanGame` for that game's install dir; while running, the
  pane shows the existing Working…/Cancel affordance.
- [x] The TUI detail view lists and handles a rescan key that calls the
  same `Session.RescanGame` for the detail row.
- [x] Full verification: `go test ./...`, `go vet ./...`, `gofmt -l .`,
  GOOS=windows/darwin builds.
- [x] Docs sweep: this issue's Outcome, `docs/log.md`,
  `docs/architecture.md`, `docs/scope.md`, `README.md` (detail-pane
  actions).

## Outcome

Shipped in `49a1803` exactly as drafted. `app.ScanGame`
(`internal/app/scangame.go`) re-discovers one game by canonical install
dir through the global scan's own source paths and precedence;
`Session.RescanGame` (`internal/ui/scan.go`) runs the identical
rediscovery → covers → online identification → cover rebind → settle
stages on the per-game op registry (Working…/Cancel in the pane), toasts
`rescanned <title>`, persists the cache, and keeps the old row on
cancellation or `ErrGameNotFound` (warning toast — pruning stays the
global scan's job). GUI: Rescan button (refresh icon) in detail action
row 1 after Game directory, placed inside the parallel session's
issue-036 two-row layout that landed mid-flight. TUI: `R` on the detail
screen, matching the games screen's library-wide `R`. ATDD red witnessed
(`tmp/test-red-035.log`: compile reds in app/ui/gui, behavior red in
tui), green in `tmp/test-green-035.log`; `go test ./...` exit 0 (32
packages) plus uncached reruns of the four touched packages, vet/gofmt
clean, windows/darwin builds OK. Docs: log.md, architecture.md,
scope.md, README.md. Left open: a title that rescans INTO a collision
gets its disambiguation suffix immediately, but a suffix that becomes
stale on the OTHER row refreshes only at the next global scan.
