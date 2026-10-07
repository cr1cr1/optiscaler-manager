---
title: DLSS backups live in the game directory
description: Update/restore rollback snapshots moved from the central data root into the game directory itself, with human-readable directory names, so backups travel with the game folder and can be recovered by hand.
---

# 022 — DLSS backups live in the game directory

## What and why

Every DLSS update/restore already backed up the current set first — but
into the central state root at
`<data-root>/dlss-backups/<sha256(installDir)[:16]>/<unix-nanos>/`. That
schema had three user-visible weaknesses: backups were invisible to
someone browsing the game folder (no manual recovery without the app),
the `sha256(installDir)` key orphaned every backup when the game folder
moved (Steam library move, drive-letter change), and backups died with
the app's data root rather than with the game.

Snapshots now live next to the DLLs they protect:

```
<gameDir>/dlss-backups/20261007-130914_dlss-310.9.1/
    nvngx_dlss.dll  nvngx_dlssd.dll  nvngx_dlssg.dll  snapshot.json
```

The directory id is the local creation time plus the backed-up Super
Resolution version in tag form (`dlss.TagVersion`, the issue-019 pill
rule), with a numeric suffix on same-second collisions — named for
humans recovering files with a file manager. Everything else about the
schema is unchanged: all-or-nothing records, verify-on-write,
verify-before-restore, digest dedup, newest-first listing, and the
issue-018 backup-less path for incomplete current sets.

Consequences:

- The `dataRoot` parameter disappeared from
  `dlss.Update/Restore/Snapshots` and the `app` wrappers — location is
  identity now.
- `classify.DirFiles` skips `dlss-backups` (like `.git`): the store sits
  inside the scanned tree, and its stale copies must never alias into
  version probing (the pill showed the BACKUP's version) or
  injection-dir resolution.
- Legacy central backups are NOT migrated and no longer listed (user
  decision: clean break, no union-read fallback). Games re-backup on the
  next update/restore anyway.

## Acceptance

- [x] An update writes the backup (dlls + `snapshot.json`) into
      `<gameDir>/dlss-backups/<id>/` and nothing under the data root.
- [x] Snapshot directory names match `<YYYYMMDD-HHMMSS>_dlss-<version>`.
- [x] Restore round-trips from the in-game location; tampered snapshots
      are still refused.
- [x] Version probing / game classification ignore `dlss-backups`
      copies (pill shows the applied version after an update).

## Outcome

Implemented as above. Commit: see below (pinned after merge).
Verification: `go test ./...` (29 packages) exit 0, `go vet`/`gofmt`
clean, `GOOS=windows`/`darwin go build ./...` OK.
