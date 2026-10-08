---
title: Move cover sharpening to store time
description: The issue-038 decode hook re-sharpens on every registry reload (registry sweep + scroll churn) and decodes on the UI thread; bake the 10% sharpen into the cached file once at fetch/override time plus a one-time migration, and remove the decode hook.
---

# 039 — Move cover sharpening to store time

## What and why

Issue 038 sharpened covers in-memory via a vendored shirei decode hook
(v0.18). User report: the whole app feels sluggish. The hook only runs
at decode, but shirei's image registry sweeps unused entries, so
scrolling re-decodes and re-sharpens covers repeatedly — and the
sub-500 KB path does it synchronously on the UI thread. Any runtime
sharpening is the wrong trade: the sharpened result is deterministic,
so it belongs in the on-disk cache, computed once ever.

Change:

- Apply the same 10% unsharp mask (RGB only, alpha untouched) to the
  cached file at store time: in `Covers.fetch` (single download seam
  for CDN/SGDB/PCGW/GOG/Wikidata/hero) right after `normalizeCover`,
  and in `Covers.SetOverride` after its normalize. Decode → sharpen →
  re-encode (same format, jpeg q90 / png), atomic temp+rename — the
  same shape as `normalizeCover`.
- One-time migration for existing caches: `New` sweeps every `*.img`
  (placeholder excluded — not an `.img`) when the marker file
  `_sharpened-v1` is absent, then writes the marker. Synchronous in
  the constructor: race-free with fetches, one slower startup ever.
- Remove the runtime path entirely: revert vendor patch v0.18 from
  vendored `images.go` (marked removed in docs/vendor-patches.md),
  delete the GUI hook install/wiring and the
  `CoversCacheDir`/`CacheDir` accessors added for it.
- Cache-hit reads (`Cover` legacy scrub) do NOT re-sharpen — the file
  on disk is already final, so reads never decode.

## Acceptance

- [ ] A fetched cover's cached bytes decode to exactly the 10% unsharp
      mask of the served image (edge test: 100/160 step → 98/162).
- [ ] A flat fetched image is byte-stable through sharpen (no-op).
- [ ] A second `Cover` call (cache hit) leaves the file bytes
      unchanged — no double sharpening, no read-time decode.
- [ ] Migration: a pre-seeded unsharpened `.img` is sharpened by `New`
      and the `_sharpened-v1` marker appears; a second `New` leaves the
      bytes unchanged.
- [ ] `sharpenRGBA` unit tests (edge math, alpha, fresh buffer) live in
      the covers package.
- [ ] Vendored `images.go` is byte-identical to upstream (v0.18 gone);
      vendor-patches.md records the removal.
- [ ] `go test ./...`, vet, gofmt clean; docs swept (issue 038 outcome
      addendum, log.md, architecture.md, scope.md).

## Outcome

Shipped as specified. The 10% unsharp mask now lives entirely in the
covers store seam: `Covers.fetch` and `Covers.SetOverride` sharpen the
cached bytes right after `normalizeCover`, and a marker-gated
(`_sharpened-v1`) one-time sweep in `New` migrates legacy caches.
Reads never decode; the GUI does no image processing at all. The
issue-038 runtime path is fully removed: vendored `images.go` is
byte-identical to upstream, the hook install/wiring and the
`CacheDir`/`CoversCacheDir` accessors are gone, and
docs/vendor-patches.md marks v0.18 REMOVED.

- Seven tests in internal/covers/sharpen_test.go: store-time sharpen
  exactness (edge 100/160 → 98/162 through the real fetch), flat
  no-op, cache-hit byte-stability, migration once-only, and the three
  `sharpenRGBA` unit pins. Behavior red witnessed
  (`tmp/test-red-039.log`).
- Verification: covers/ui packages green uncached; full-suite green
  verified in a clean git worktree at the feat commit because the main
  tree hosts the foreign session's in-flight issue-040 pulse-ticker
  reds (their caret/spinner/title-edit files change under the test
  runner). vet/gofmt clean; GOOS=windows/darwin builds OK.

Commit: d6bf0d0

**Collision note.** Staging d6bf0d0 raced the foreign session's
issue-040 work: their in-flight model.go hunk (pulseFrames,
forwardSessionEvents) landed in the diff between hunk inspection and
staging and rode the commit. They noticed and committed around it
("rode the foreign session's commit d6bf0d0... content intact" in
5f806d7), so the tree builds from 5f806d7 onward; d6bf0d0 alone lacks
pulseWanted and does not build standalone. A subsequent amend of mine
mistakenly targeted their docs-pin commit; it was fully undone via
reset to e21b35d (reflog orphan 0e00f40), leaving every foreign hash
valid. Final full-suite verification ran in a clean worktree at
e21b35d: 32/32 packages green with both issues' code combined.

