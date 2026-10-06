---
title: Fork archive layouts and fork-switch dated backups
description: Derive the install/backup file set from each distribution's own archive listing (forks ship different root layouts, e.g. DLSSNR's OptiScaler/ subdir and missing fakenvapi), and preserve the old distribution's files in a <repo>.YYMMDD dir inside the game folder when a switch crosses forks.
---

# 010 — Fork archive layouts and fork-switch dated backups

## What and why

Two gaps surfaced now that installs can come from selectable forks
(issue 5):

1. **The hard-coded required set rejects real forks.** `buildPlan`
   demands `optiscaler.dll` + `fakenvapi.dll` + `fakenvapi.ini`
   (upstream 0.9.4 ground truth). The bundled DLSSNR fork ships **no
   fakenvapi at all**, so installing it fails with "bundle is missing
   required file". Forks also lay files out differently: DLSSNR keeps
   `OptiScaler.dll`/`OptiScaler.ini` at the archive root but its support
   DLLs under an `OptiScaler/` subdir (paths the mod expects relative to
   the game dir — they must install verbatim, never stripped). The fix
   is the user's rule: **each distribution's archive listing is its own
   exact file set** — the only universal requirement is the injector
   dll; everything else installs as listed, nested paths preserved. A
   bundle without `OptiScaler.ini` gets the curated defaults tracked as
   a created file instead of failing the install.
2. **Switching forks silently discards the old distribution.** A
   version switch across forks uninstalls the old fork's files
   (created → deleted, overwritten → restored) with no user-visible
   trace. Now the chain detects a fork change (manifest fork ≠ active
   source) and **moves** the old distribution's file set into
   `<installDir>/<repo-name>.YYMMDD/` (repo segment of the old fork's
   slug, local date; `-2`, `-3`… on same-day collisions), preserving
   nested relative paths. Overwritten entries still restore their
   SHA-verified pre-install originals after their current bytes are
   relocated; foreign-modified files are still refused, never moved.

Same-fork release switches keep the existing internal SHA-verified
backup (rollback-restorable) — no dated dirs pile up on version bumps.
External (unmanaged) installs are unchanged: the adopt path already
backs up every overwritten file SHA-verified; with no manifest there is
no reliable old file set to relocate.

## Acceptance

- [x] `buildPlan` requires only `OptiScaler.dll`; a DLSSNR-shaped
      listing (no fakenvapi, nested `OptiScaler/*`, docs at root) plans
      successfully with nested destinations preserved.
- [x] Installing a DLSSNR-shaped zip succeeds: `dxgi.dll` at the
      injection dir root, support DLLs under `OptiScaler/`, docs
      verbatim; a bundle without `OptiScaler.ini` still gets the curated
      ini, tracked in the manifest.
- [x] `installer.Uninstall` with a relocate dir moves hash-matching
      created files and overwritten files' current bytes into it
      (relative paths preserved), restores pre-install originals, and
      still refuses foreign-modified files.
- [x] `SwitchVersion` across forks leaves `<repo>.YYMMDD` inside the
      game's injection dir holding the old fork's files; same-fork
      switches leave no dated dir.
- [x] `go test ./...`, `go vet`, `gofmt` clean; docs updated.

## Outcome

Done. `buildPlan` validates the injector only and installs every other
archive member verbatim (nested paths preserved); `applyCuratedINI`
tracks the curated ini as created when the bundle ships none;
`installer.UninstallWithOptions` (`UninstallOptions.RelocateDir`) moves
hash-matched bytes into the dated dir with originals restored and
foreign modifications refused; the switch chain detects a fork change
(manifest fork vs active source) and relocates into
`<injection-dir>/<repo>.YYMMDD` (`forkBackupDir`, `-2`… collision
suffixes, `s.now` clock seam), toasting the dir name. Design decisions
(location, repo-segment naming, automatic trigger, same-fork internal
backup) confirmed with the user. Evidence: the real cached DLSSNR zip
(injector at root, support DLLs under `OptiScaler/`, no fakenvapi) vs
the flat upstream 7z. TDD reds witnessed before the implementation;
full suite, vet, gofmt green. Commit `3dc930a`.
