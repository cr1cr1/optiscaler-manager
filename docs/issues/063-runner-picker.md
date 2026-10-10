---
title: Runner picker for umu launches (global + per-game)
description: The Proton build for umu launches is user-selectable from the detected installations (Steam compatibilitytools.d, Bottles/Lutris runners, Heroic tools/proton) — global default plus per-game pin, GUI dropdowns and a TUI picker modal.
status: done
---

# 063 — runner picker for umu launches

## What and why

The runner was always auto-resolved (newest matching the prefix's
launcher, else global newest). Users need to pick the build themselves
— e.g. a known-good GE-Proton instead of the newest UMU-Proton — per
the launcher's "choose prefix and runner from existing installations"
configuration model. Only umu launches use it; owner-CLI launches run
on the launcher's own configured runner.

## Acceptance

- [x] Settings: `UmuRunnerPath` (global default) + `RunnerOverrides`
  (per-game pins), precedence per-game → global → Auto (newest
  detected), persisted; the LaunchRoute precedent.
- [x] GUI: "Runner" dropdown in Settings (buffers until Apply) and in
  the detail panel (applies immediately); rows render "Launcher:
  runner" from the detected builds, Auto row first, Custom row for an
  undetected pin.
- [x] TUI: `n` on settings opens the global runner picker modal, `n`
  on the detail screen the per-game one (badges + 20-row windowing,
  the prefix picker's machinery).
- [x] The router passes the resolved pin to `ResolveProtonPath`
  (pinned wins verbatim); pinned PROTONPATH covered by a cmd test.
- [x] `go vet`, `go test ./...` green; darwin/windows cross-compile OK.

## Outcome

Shared composition in internal/ui (`UmuRunnerRows`/`UmuRunnerLabel`,
mirroring the prefix pickers' `UmuPickerRows`/`UmuPickerLabel`) fed by
a new `Deps.UmuRunners` provider (nil = production disk scan). The GUI
route picker was generalized into `optionsPicker` (options as a
function, so the detail dropdown scans on the opening frame and still
renders fully); the TUI reuses the prefixPick modal. Commit:
8e9e158.
