---
title: Posters lost when the cache file vanishes — boot never re-resolves dead CoverPaths and shirei LoadImage pins a pixel-less zombie
description: Issue 042's hi-res migration deleted 97 cached covers; boot hydration trusts the persisted CoverPath (dead file → filler), and even after a rescan re-downloads the file, LoadImage's registered miss-entry is returned forever. Fix both layers.
---

# 046 — Posters lost when the cache file vanishes: boot self-heal + LoadImage miss retry

## What and why

User report: after restarting the app (first run with issue 042's
`_hires-v1` migration, which deleted 97 lo-res cached covers), the vast
majority of posters show only the generic card filler; per-game rescans do
not bring them back.

Reproduced and root-caused on the user's machine (2026-10-08):

1. **Boot never re-resolves dead paths.** Warm-boot hydration
   (`Session.Start`) trusts the persisted `CoverPath`; the file it points
   to is gone after the migration, so cards render filler until a scan.
   Cache-dir evidence: 97/113 rows with `CoverPath` pointing at deleted
   files. The design assumption "persisted CoverPath ⇒ file exists" broke
   the moment a migration deleted cache files.
2. **LoadImage pins a pixel-less zombie.** The vendored shirei registry
   (`LoadImage`) registers and file-caches a pixel-less `ImageData` when
   the file is missing; the visible card touches that entry every frame,
   so the sweep never reclaims it. When a rescan re-downloads the cover to
   the SAME path, the registry keeps returning the zombie — verified on
   disk: rescans DID re-download (5006530.img 18:06, 860510.img 18:08,
   2050650.img 17:58) while the cards kept showing filler.

Fixes:

- **Vendor patch v0.21** (`images.go`): a missing/unreadable file
  (`len(content) == 0`) is not registered and not file-cached — a
  transient pixel-less image is returned for this frame only, so the next
  frame retries the (cheap) stat+read and decodes once the file lands.
  Present-but-undecodable files keep the cached behavior (no per-frame
  hammering); the >64MB async-read in-flight path is unaffected (nil
  content while reading → transient → retried after the read caches).
- **Boot self-heal** (`Session.Start`): after warm-boot hydration, rows
  whose `CoverPath` file no longer exists are re-resolved in a background
  pass (same `resolveCover` chain as a scan — override pin first, appid,
  title search), then state is updated, the cache persisted, and a poke
  event emitted so frontends repaint. Skipped when online lookups are off
  (network work) or `Covers` is nil. Live files are never re-fetched.

## Acceptance

- [ ] Red witnessed: `TestLoadImageRecoversWhenFileAppears` (gui) and
      `TestStartHealsDeadCoverPaths` (ui) fail before the fix.
- [ ] After the fix: re-downloading a cover to a previously-missing path
      renders the poster without an app restart; a warm boot with dead
      `CoverPath` files re-fetches them in the background (online lookups
      on) and leaves live files untouched.
- [ ] Vendor guard covers the v0.21 marker; docs/vendor-patches.md entry.
- [ ] Full `go test ./...` green, vet/gofmt clean, docs swept.

## Outcome

Filled in when the issue closes: what actually happened, the commit
hash, anything left open.
