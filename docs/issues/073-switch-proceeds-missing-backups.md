---
title: Version switch proceeds despite missing uninstall backups (warn, don't abort)
description: Switching a game's OptiScaler version hard-failed with "N backup file(s) missing, originals unrestorable" when the install's backup blobs were gone — the fork-switch path skips the consent gate by design, and the same-fork path dead-ended the chain; the switch now warns and proceeds.
---

# 073 — Version switch proceeds despite missing uninstall backups

## What and why

User report: switching a game (UNCHARTED) to the selected non-default fork's
version failed with
`Failed: uninstall a8032d38cb43fc53: 8 backup file(s) missing, originals unrestorable: […]`.
Directive: the warning is fine, but the switch must proceed regardless.

Root cause: the switch chain's uninstall leg calls
`runUninstallOpt(gameDir, relocateDir, false)`. On missing backups two
broken paths existed:

- Fork switch (`relocateDir != ""` — the user's case: installed fork ≠
  active fork): the `ConfirmMissingBackup` consent gate is skipped by
  design (its continuation cannot relocate), so the typed error fell to
  `opFailed` and the chain aborted before anything ran.
- Same-fork switch: the gate fired, but the chain treated the pause
  sentinel as a generic failure and settled "switch failed"; accepting the
  modal ran a *standalone* uninstall — the game was left uninstalled and
  the version never switched.

Missing backups mean the pre-install originals cannot be restored — for the
affected files the mod bytes are simply removed (the game never shipped
most of them, or the originals are lost either way). Aborting protects
nothing the user can still get back; it only blocks the requested switch.

Fix: the switch chain pre-flights the manifest for missing backup blobs
(new `installer.MissingBackupPaths`), surfaces one warning (zerolog + warn
toast naming the count), and runs the uninstall leg with
`AllowMissingBackup=true`. The standalone Uninstall keeps its
`ConfirmMissingBackup` consent modal (issue 050) unchanged.

## Acceptance

- [x] Switching versions on a game whose install manifest references
      missing backup blobs completes: old build removed (missing originals
      dropped), chosen version installed, preserved OptiScaler.ini restored,
      exactly one warning surfaced (toast + log), no confirmation modal,
      no "switch failed".
- [x] Same behavior for a fork switch (relocation still applies).
- [x] Standalone Uninstall still pauses on the ConfirmMissingBackup modal;
      accepting it proceeds as before (issue 050 semantics).
- [x] `go test ./...`, `go vet`, `gofmt` green; docs updated.

## Outcome

Implemented as designed. Both red paths witnessed before the fix: the
fork-switch settled `switch failed: uninstall …: 1 backup file(s) missing`
(the user's exact line) and the same-fork switch paused on the consent
modal while the chain settled `switch failed: ui: install paused for
consent` — accepting it would have run a standalone uninstall, leaving the
game uninstalled and unswitched.

- `internal/installer/rollback.go`: `MissingBackupPaths` mirrors the
  uninstaller's missing-backup condition exactly (installed bytes still
  match the manifest, backup blob gone; vanished files and refusals
  excluded).
- `internal/ui/switch.go`: the chain pre-flights `missingBackups`, warns
  once (zerolog with the paths + warn toast with the count), and runs the
  uninstall leg with `AllowMissingBackup=true`. Standalone Uninstall keeps
  the issue-050 consent modal; the modal's continuation is unchanged.
- Red-first tests: `TestSwitchMissingBackupsWarnsAndProceeds` (same-fork:
  settles "switched to v0.10.0-test", no modal, warning toast, custom ini
  preserved, new bundle in place) and `TestForkSwitchMissingBackupsProceeds`
  (old fork's bytes still relocated to the dated dir, upstream installed).
- Docs: scope.md (backup paragraph now states the switch semantics).
- `go test ./...` 35/35 ok, vet + gofmt clean.

Commit: 26c6eb3.
