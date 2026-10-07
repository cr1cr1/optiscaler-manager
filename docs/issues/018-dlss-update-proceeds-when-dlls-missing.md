---
title: DLSS update/restore proceeds when the current DLLs are missing
description: A partial or absent current NVIDIA set no longer blocks DLSS update/restore — it warns and proceeds without a rollback backup.
---

# 018 — DLSS update/restore proceeds when the current DLLs are missing

## What and why

Switching DLSS versions (update or restore) failed hard when any of the
three current NVIDIA DLLs was absent from the game's injection directory:
`Update`/`Restore` hard-gated on `requireFiles` ("dlss: nvngx_dlssd.dll is
missing") and `backup` errored the same way. The only thing the current
DLLs are needed for is the pre-op rollback backup — an incomplete set has
no complete-set rollback value (snapshots are all-or-nothing), so the
correct behavior is: log a warning and proceed, installing/restoring the
complete target set, without writing a backup.

## Acceptance

- [x] `dlss.Update` over a partial current set succeeds: every member —
      including the missing ones — lands at the target version, no
      snapshot is written, the returned `Snapshot` is the zero value.
- [x] `dlss.Restore` over a partial current set succeeds and writes no
      backup of the incomplete set.
- [x] The incomplete-set case logs a zerolog warning naming the missing
      member; the op settles as a normal success.
- [x] Complete-set behavior is unchanged: backup-first, dedup, verified
      rollback on failure.
- [x] `go test ./...`, `go vet`, `gofmt` green; docs updated.

## Outcome

Implemented as designed.

- `internal/dlss/dlss.go`: the `requireFiles` hard gates in `Update` and
  `Restore` are gone; both call the new `backupIfComplete`, which backs up
  a complete current set as before but downgrades an incomplete one to a
  warning + zero `Snapshot`. The zero snapshot neutralizes the
  rollback-on-failure leg (`restoreFiles` over zero files is a no-op) —
  there was nothing restorable to begin with. `requireFiles` survives as
  the `Complete` pill gate (badge vs. interactive control), unchanged.
- Frontends needed no changes: the op settles through the existing
  `runDLSSOp` success path; the warning rides the log (and the log
  console, per issue 011). The restore confirmation copy now says "A
  complete current set is backed up first."
- Tests flipped from refusal to proceed:
  `TestUpdateInstallsWhenCurrentDLLsMissing`,
  `TestRestoreInstallsWhenCurrentDLLsMissing` (internal/dlss),
  `TestUpdateDLSSMissingDLLProceeds` (internal/ui),
  `TestTUIDetailUpdateDLSSInstallsOverMissing` (internal/tui).
- Comments updated in `internal/ui/dlss.go` and `cmd/dlss.go` (the
  "never adds a missing DLL" contract is gone); `docs/scope.md` DLSS
  bullet updated.

Commit: 014be06.
