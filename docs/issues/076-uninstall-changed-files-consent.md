---
title: Uninstall warns and proceeds on changed files
description: Standalone uninstall gains a consent gate for foreign-modified files instead of hard-blocking forever, mirroring the issue-050 missing-backup gate.
---

# 076 — Uninstall warns and proceeds on changed files

## What and why

A standalone uninstall whose tracked files no longer match the manifest
(foreign modification — e.g. the user extracts another OptiScaler fork's
files over the managed install, preserving archive mtimes) aborts with
`RefusedError`, and no frontend offers a way past it: the game can never
be cleanly uninstalled from the UI, and every retry re-fails on the same
file. Missing backups got a warn-and-proceed confirmation in issue 050
(CLI `--force`); changed files need the same escape hatch.

Root-cause evidence on the reporter's machine (The Witcher 3 GOTY):
`libxell.dll` on disk was byte-identical to the jlrouzies NR fork v0.8.92
archive's copy (zip-internal mtime preserved), while the manifest recorded
the managed official v0.9.4 bytes. The hash check worked as designed; the
missing recovery path is the defect.

Design: `UninstallOptions.AllowChanged` consents to removing
foreign-modified bytes — created files take the normal created fate
(delete, or relocate when `RelocateDir` is set, so the bytes stay
recoverable), overwritten files drop the current bytes and restore the
SHA-verified pre-install backup as usual. The standalone-uninstall UI
pauses with a `ConfirmChangedFiles` dialog naming the files; accepting
resumes with the override. The version-switch chain keeps refusing
(relocate path, issue-010 semantics unchanged). CLI gains
`--force-changed`.

## Acceptance

- [x] `UninstallWithOptions` with `AllowChanged` removes a changed created
      file and restores a changed overwritten file from its backup;
      without it the `RefusedError` behavior is byte-for-byte unchanged.
- [x] Standalone uninstall from the session pauses with
      `ConfirmChangedFiles` naming the changed files; declining leaves
      every byte in place, accepting completes the uninstall.
- [x] `optiscaler-manager uninstall --force-changed` proceeds; without
      the flag the error text hints at it.
- [x] `go test ./...`, `go vet`, gofmt clean.

## Outcome

Shipped as designed. `UninstallOptions.AllowChanged` +
`ConfirmChangedFiles` gate in the session (mirroring issue 050's
ConfirmMissingBackup), CLI `--force-changed`; switch-chain refusal
semantics untouched. Tests:
`TestUninstallAllowChangedRestoresAndRemoves` (installer),
`TestUninstallChangedFilesConfirmFlow` (session: block → decline →
consent → original restored). Full suite 35/35 green, vet + gofmt clean.
Flake noted: `TestVersionDropdownMenuOffersLatest` timed out once on a
20s wall-clock row wait in an untouched path; rerun green (pre-existing
load flake).

Commit: THIS_COMMIT.
