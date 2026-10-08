---
title: Fetch hi-res Steam cover art (_2x variant) and resample at Lanczos quality
description: Steam's CDN now serves library_600x900.jpg at half resolution (300x450); the true 600x900 art is the _2x variant. The fetch chain tries _2x first (portrait and hero), a marker-gated migration drops narrow CDN-cached covers for re-fetch, and the GUI's idle resampler moves from bilinear to Lanczos.
---

# 042 — Fetch hi-res Steam cover art (_2x variant) and resample at Lanczos quality

## What and why

User report: the detail-pane poster looks like an upscale of the card
poster; it must render at the highest quality available.

**Root cause** (verified against the live CDN and the local cover
cache): Steam's CDN now serves `library_600x900.jpg` at **300x450** for
every probed appid — the true 600x900 art lives at
`library_600x900_2x.jpg`. Cards (~174px wide) downscale the 300px file
and look fine; the detail pane (~350-440px) *upscales* it — exactly the
reported symptom. The local cache was ~90% 300x450 files.

**Fix**:
1. The CDN fetch tries `_2x` first, falling back to 1x, for both the
   portrait and the hero banner (hero_2x crops to a 826x1240 portrait
   instead of 413x620) — shared `fetchPortrait`/`fetchHero` helpers.
2. A marker-gated (`_hires-v1`) one-time migration drops cached
   CDN-named covers narrower than 600px so they re-fetch at full
   resolution; source-prefixed files (sgdb_/pcgw_/gog_) keep their own
   sizes. Mirrors the issue-039 sharpen migration's shape.
3. GUI startup sets `ScaleIdleFilter = transform.Lanczos`
   (`configureImageQuality`): the software render path's steady-state
   resample is no longer bilinear. bild is promoted to a direct
   dependency.

Foreign-test touch: `TestNewMigratesLegacyCacheOnce` (issue 039) seeded
a 64x96 numeric-named file — exactly what the hires migration drops;
the fixture is now 600x900 (sharpen math is width-independent).

## Acceptance

- [x] `TestCoverPrefers2xArt` — _2x requested first, its bytes cached
      (red before: only 1x was fetched).
- [x] `TestCoverFallsBackTo1xWhen2xMissing` — 404 on _2x falls back.
- [x] `TestHiresMigrationDropsNarrowCDNCovers` — narrow numeric-named
      files dropped; wide and source-prefixed files kept; marker gated.
- [x] `TestIdleImageFilterIsHighQuality` — idle resampler is Lanczos.
- [x] Full `go test ./...` green; `go vet`/`gofmt` clean.

## Outcome

Done. TDD red witnessed (1x-only fetch). The user's existing cache
self-heals on next launch via the migration. Commit: 71c4cd0.

Deferred: the GPU render path scales in hardware (GL linear) — no
Lanczos equivalent without shader work; acceptable since the software
path (the fallback) now has the best CPU resampler. shirei's
scaled-image cache still paints one nearest-neighbor frame plus a
~120ms wait window on one-shot size changes before upgrading to the
idle filter; with layout animations disabled (v0.13) this is a brief
flash at most — revisit only if it proves visible.
