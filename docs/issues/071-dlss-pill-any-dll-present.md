---
title: DLSS pill clickable for any game with a DLSS DLL, not just complete sets
description: The update/restore control degraded to a dead static badge unless all three NVIDIA DLLs were present; most real games ship only nvngx_dlss.dll, so the pill looked normal but ignored clicks for the majority of the library.
---

# 071 — DLSS pill clickable for any game with a DLSS DLL

## What and why

User report: "clicking the `dlss <version>` pill to update DLSS / show the
previous-versions menu is gone — pill is there, click does nothing, all
games, card and detail panel."

Root cause: `6024fe4` added `!e.DLSSReady` to the GUI pill gate
(`dlssControl`). `DLSSReady` = `dlss.Complete` = the complete three-file
NVIDIA runtime set (`nvngx_dlss.dll`, `nvngx_dlssd.dll`, `nvngx_dlssg.dll`).
Most games ship only one or two of these, so their pill silently became a
static badge that looks identical to the control. Real-library evidence:
34 rows render a DLSS pill, only 6 are complete-set — 28 dead pills.
The backend has tolerated incomplete sets since issue 018 (update proceeds,
installs the complete target set, skips the rollback backup), so the gate
served no protective purpose — it only hid the feature.

Fix: interactivity follows the pill's existence (a DLSS DLL is present,
versioned or bare) in both frontends; `DLSSReady` stays as an informational
row field, no longer a gate. Restore still requires backups (which only
complete sets produce) — the arrow menu already no-ops with zero snapshots.

## Acceptance

- [x] GUI: a game with a DLSS pill but an incomplete set (missing
      nvngx_dlssd.dll / nvngx_dlssg.dll) renders the interactive control on
      card AND detail panel; clicking the version area runs the update and
      lands the complete set.
- [x] GUI: card height estimation (`componentPillW`) mirrors the control
      for incomplete-set games (no card-height regression).
- [x] TUI: the `u`/`p` hints are no longer dimmed for an incomplete-set
      game with a DLSS component.
- [x] Complete-set behavior unchanged: backup-first update, restore menu,
      busy-game static pill.
- [x] `go test ./...`, `go vet`, `gofmt` green; docs updated.

## Outcome

Implemented as designed.

- `internal/gui/dlss.go`: `dlssControl` gate drops `!e.DLSSReady` — the
  control follows the pill's existence (a DLSS DLL present, versioned or
  bare). `internal/gui/grid.go`: `componentPillW` mirrors it.
- `internal/tui/model.go`: `hasDLSS` now means "carries a DLSS component"
  (was: `DLSSReady`), so the u/p hints and the restore picker stop
  excluding partial sets; restore still requires an existing backup.
- `internal/ui/rows.go`: `DLSSReady` documented as informational.
- Red-first tests: `TestDLSSControl_IncompleteSetIsInteractive` (pill
  renders + click updates a lone-dll game to the complete published set,
  no snapshot per 018, estimator parity); TUI flips in
  `TestDetailViewDLSSHintsTrackComponents` (incomplete set enabled, only
  DLSS-less rows dimmed) and `TestTUIRestorePickOpenForIncompleteSetWithBackup`.
  `TestDLSSControl_RendersOnCardsAndPanel` negative case corrected to a
  truly DLL-less game; the bare-pill partial-set case asserts interactive.
- Docs: scope.md (pill gate + three-file bullets), README.md.
- `go test ./...` 35/35 ok, vet + gofmt clean.

Commit: 3ca87eb.
