---
type: explanation
---

# Safety model

The installer mutates directories owned by games. These invariants are
non-negotiable; the fault-injection tests in `internal/installer` exist to
keep them true.

## Invariants

1. **No write into a game directory until the bundle is staged, path-sanitized,
   required-file validated, and hash-manifested.**
2. **A manifest is persisted before the first destructive write.**
3. **An original file's bytes are verified in the backup before it is
   overwritten.**
4. **Rollback and uninstall are idempotent** — safe to re-run after a crash at
   any step.
5. **Uninstall deletes only files whose current SHA-256 matches the manifest**;
   anything else is refused and surfaced to the user (no silent deletion of
   foreign bytes — `dxgi.dll` et al. may be native game files).

## Manifest

JSON, external store, one per install (keyed by canonical install dir):

```
id, schemaVersion, status, gameRoot, installDir (canonical),
requestedVersion, resolved {assetName, version, sha256},
overwritten[] {path, backupRelPath, preSHA256, installedSHA256},
created[]    {path, sha256},
createdDirs[], timestamps, lastError
```

`requestedVersion` and `resolved` are separate fields: rate-limit or explicit
substitution must never silently change what the user asked for.

## State machine

```
in_progress → committed
           ↘ failed → rolled_back
```

`planned` is in-memory only. On startup, manifests in `in_progress`/`failed`
are surfaced with repair / rollback / retry choices; nothing is auto-deleted.

## Backups

External per-install backup directory keyed by manifest ID, holding the
original bytes of every overwritten file (relative-path preserved). No
content-addressed store in v0.1 (dedup is not a correctness problem).

## Archive validation

Third-party archives are hostile input. Before any write: reject absolute
paths, `..` traversal, drive roots, UNC paths, reserved names, symlink/hardlink
metadata, case-folded duplicate targets, dir/file conflicts, decompression-bomb
totals, and unexpected filenames. Extract to staging; verify required files and
hashes; only then copy into the game dir.

## Fault-injection scope (exactly five tests)

1. `TestRejectsUnsafeArchive` — traversal/absolutes rejected at plan time.
2. `TestBacksUpBeforeOverwrite` — original bytes verified in backup first.
3. `TestRecordsCreatedAndOverwritten` — manifest reflects reality after commit.
4. `TestRollbackFromInProgress` — restore originals, delete matching created.
5. `TestUninstallRefusesChangedFile` — SHA mismatch → refuse, report.

Deliberately absent: syscall-combinatoric fault matrices, content-addressed
backup stores, clock interfaces. Ceiling named: if real-world crash reports
appear, deepen injection at the observed boundary (`ponytail`).
