---
title: Uninstall blocked by missing backup — confirm-to-proceed removal
description: A missing optiscaler-backups entry hard-fails the whole uninstall; pause with a confirmation and let the user proceed with removal.
---

# 050 — Uninstall blocked by missing backup — confirm-to-proceed removal

## What and why

User report (2026-10-08): uninstalling an old semi-manual (adopted)
OptiScaler install that had been switched to the optiscaler-dlssnr fork
fails outright:

```
WRN Failed: uninstall cea58c7827e6f3a7: backup unreadable: open
.../optiscaler-backups/files/amd_fidelityfx_framegeneration_dx12.dll:
no such file or directory
```

Root cause: `installer.UninstallWithOptions` hard-fails on the FIRST
unreadable backup entry (`rollback.go` `hashFile(backup)` error path) and
aborts the entire uninstall, leaving the user stranded — the mod stays
installed forever because one backup file is gone. Whatever removed the
backup (fork-switch relocation era, manual cleanup) is unrecoverable, so
the block is permanent.

Fix: a missing backup must not block. The installer collects ALL missing
backups and returns a typed `MissingBackupError`; the UI pauses with a
confirmation naming the affected files, and accepting proceeds with
removal (the unrestorable installed bytes are deleted, the lost original
is simply not restored). CLI mirrors the existing `--force` precedent.
The fork-switch chain keeps failing loudly (no consent prompt mid-chain).

## Acceptance

- [ ] Uninstall over a missing backup returns a typed
  `MissingBackupError` listing every missing entry (no fail-fast on the
  first), with manifest progress persisted.
- [ ] `UninstallOptions.AllowMissingBackup` completes the uninstall: the
  installed bytes are removed, the manifest and backup store are deleted,
  the lost original is not restored.
- [ ] GUI/TUI: standalone uninstall pauses with a
  `ConfirmMissingBackup` prompt naming the files; declining leaves
  everything untouched; accepting completes the removal.
- [ ] CLI `uninstall` hints at `--force`; `uninstall --force` proceeds.
- [ ] Fork-switch chain (`runUninstallOpt` with relocateDir) behavior
  unchanged: typed error surfaces, no confirmation mid-chain.
- [ ] `go test ./...` green, vet/gofmt clean.

## Outcome

Commit: TBD.
