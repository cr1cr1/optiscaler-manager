---
title: Every touched original is backed up in the game dir, with a 100MB consent gate
description: OptiScaler install backups move from the central store into the game directory (clean break), and any operation whose pending backup exceeds 100MB pauses for explicit user confirmation; decline or non-interactive CLI aborts before a single byte is touched.
---

# 023 — Universal original backup in the game dir + 100MB consent gate

## What and why

Issue 022 moved DLSS rollback snapshots into the game directory. This
issue generalizes the principle: **no original file is modified without a
backup in the game directory first**, and any operation whose pending
backup exceeds **100MB total** pauses for explicit consent. Declining —
or a non-interactive CLI that cannot ask — aborts the operation before a
single byte is touched.

Two changes:

1. **OptiScaler install backups move in-dir.** Overwritten originals
   (pre-existing hooks, adopted external installs) currently copy to the
   central `<state>/backups/<manifest-id>/files/<rel>`. They now copy to
   `<installDir>/optiscaler-backups/files/<rel>` — same layout, same
   per-file verify-on-write, same manifest record (`BackupRelPath` is
   already relative; only the root changes). Uninstall/rollback read
   from the in-dir root and remove the tree when the manifest settles.
   `classify.DirFiles` skips `optiscaler-backups` (same aliasing hazard
   as issue 022: a backed-up `dxgi.dll` must not read as an active
   external install).

   **Clean break (user decision):** no union-read, no migration.
   Installs committed before this change have their originals ONLY in
   the central store; uninstalling such a game after this change cannot
   restore those originals (the uninstall errors on the missing backup,
   leaving our files in place — nothing is silently lost, the game
   keeps working with OptiScaler installed). The legacy central
   `<state>/backups/` tree becomes unreachable by the code
   (`store.BackupDir` is deleted with its last caller) and can be
   removed by hand.

2. **100MB consent gate.** Before an op writes its backup, it computes
   *planned new bytes*; over `100 * 1024 * 1024` the op refuses with a
   sentinel error BEFORE backing up or modifying anything:
   - **DLSS update/restore** (`dlss.Update`/`dlss.Restore`): planned
     bytes = 0 when the current set is incomplete (issue 018: no
     backup) or an identical snapshot already exists (dedup: no copy);
     otherwise the sum of the three current DLLs (~115MB — the first
     backup of a game prompts, dedup ping-pong stays silent).
   - **OptiScaler install** (`installer.Install` after planning): sum
     of the pre-existing target files' sizes (usually a few MB of hook
     DLLs; adopting a large external install can trip it).

   The sentinel carries the byte count; the session maps it to a new
   `ConfirmLargeBackup` kind whose message names the operation and the
   size. Accept re-dispatches the op with an override flag (the
   EAC/stale-cache resume precedent); decline aborts. All frontends
   route through `ui.Session`: GUI/TUI render the generic confirm
   modal/box unchanged; the CLI's existing `EvConfirm` gate prompts on
   a TTY and aborts non-interactively.

## Acceptance

- [x] Install writes overwritten-original backups to
      `<installDir>/optiscaler-backups/files/<rel>`, hash-verified as
      today; uninstall restores from there and removes the tree;
      interrupted-install rollback reads there too.
- [x] `store.BackupDir` and every central-backup reference are gone;
      `classify.DirFiles` skips both `dlss-backups` and
      `optiscaler-backups`.
- [x] A DLSS update whose planned backup exceeds 100MB pauses with a
      size-naming confirmation and touches nothing on decline; a dedup
      hit (identical snapshot present) does not prompt.
- [x] An OptiScaler install whose overwrites exceed 100MB pauses the
      same way; accept resumes at the same resolved version.
- [x] CLI on a TTY prompts y/N; non-interactive CLI aborts with a
      message; nothing is modified in either decline path.
- [x] Docs: scope.md, architecture.md, README.md, log.md.

## Outcome

Implemented as specced. Consent flags (`eacOK/cachedOK/largeOK`)
collapsed into one `installConsent` struct that accumulates across gates
via `Confirmation.consent`, so a later gate never re-asks an earlier
one; `app.MaxBackupNoConfirm` is the single policy owner
(`installer.defaultMaxBackupNoConfirm` mirrors it for direct library
callers). ATDD reds witnessed per slice (backup path + DirFiles leak at
runtime; both gates via temporary disable). Commit: `20ec774`.
Verification: `go test ./...` (29 packages) exit 0, `go vet`/`gofmt`
clean, `GOOS=windows`/`darwin go build ./...` OK.
