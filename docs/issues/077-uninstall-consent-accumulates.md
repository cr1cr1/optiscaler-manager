---
title: Uninstall consent gates accumulate across pauses
description: The changed-files and missing-backup uninstall gates ping-pong forever when both conditions hold because each resume drops the other consent.
---

# 077 — Uninstall consent gates accumulate across pauses

## What and why

Issue 076's changed-files gate loops forever when an uninstall hits
BOTH consent conditions (foreign-modified file AND missing backup
store — the exact Witcher 3 GOTY state that motivated 076): the
installer surfaces `MissingBackupError` first, then `RefusedError` on
the consented retry, but each `AnswerConfirm` resume re-enters
`runUninstallOpt` with only its own override set — the other consent is
dropped, so the other gate fires again, ad infinitum (user log: the
"restoring original with consent" warning repeating every second as they
click Proceed on alternating dialogs).

Installs already solve this: `Confirmation.consent` accumulates
`installConsent` overrides across the EAC/cached/large-backup gates.
Mirror that for uninstall: an `uninstallConsent{missingOK, changedOK}`
carried on the `Confirmation` and OR-ed in on each accept, so
gate 1 → gate 2 → done, exactly two dialogs.

## Acceptance

- [x] Changed file + lost backup: uninstall pauses on one gate, then the
      other, then completes — accepting both never re-prompts; the
      foreign bytes are dropped and the row leaves committed.
- [x] Single-condition flows (076 changed-only, 050 missing-only) behave
      exactly as before.
- [x] `go test ./...`, `go vet`, gofmt clean.

## Outcome

Shipped as designed: `uninstallConsent{missingOK, changedOK}` carried on
`Confirmation.uconsent` (mirroring `installConsent`), OR-ed in by each
accepted gate; `runUninstallOpt` re-pauses carrying the accumulated
consent. `TestUninstallChangedAndMissingBackupGatesChain` witnessed the
red precisely ("a third gate fired after both consents") — and first
corrected a wrong assumption: the changed-files gate fires FIRST (the
refusal check precedes the backup probe in the overwritten loop). Full
suite 35/35 green, vet + gofmt clean.

Commit: ba90d63.
