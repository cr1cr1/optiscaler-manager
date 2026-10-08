---
title: Sharpen cover posters 10% once at image-load time
description: Covers are scaled live at render, never resized on disk — apply a 10% unsharp mask to the in-memory decoded image, once per load, not per frame.
---

# 038 — Sharpen cover posters 10% once at image-load time

## What and why

Grid/detail posters look soft because covers are downscaled live at
render. Investigation of where the resize happens:

- **On disk**: never resized. `covers.fetch` stores the downloaded bytes
  as-is; `normalizeCover` (internal/covers/covers.go) only center-crops
  to ~2:3 when the aspect is off — no scaling.
- **At render**: every frame. GUI `coverArt` (grid card, list row,
  detail pane) calls shirei's `ImageFill(path)`, and the soft renderer
  scales the pixels onto the surface each frame. The decode itself
  happens **once per path per process** in vendored `LoadImage`
  (`vendor/go.hasen.dev/shirei/images.go`), which caches the RGBA in the
  image registry (`res.imageKeys`) and file-content cache.

So the live-at-display branch applies: sharpen the in-memory cached
image once on load, never on the render path, never on disk.

Spec:

- Unsharp mask, amount 0.10, per RGB channel (alpha untouched):
  `out = clamp(p + 0.10 · (p − boxBlur3x3(p)))`. Pure function
  returning a new RGBA (never mutates a shared buffer).
- Applied inside vendored `LoadImage` at both decode sites (sync small
  files and the >500 KB async path) via a new decode hook — vendor
  patch **v0.18** with marker comment, docs/vendor-patches.md entry,
  and a guard test like the existing ones in internal/gui/csd_test.go.
- GUI installs the hook once at startup; it sharpens only paths under
  the covers cache dir (downloaded covers + override posters). Icons
  and any other images pass through untouched.
- Disk bytes stay pristine: a refetch (rescan, rebinding) re-decodes
  and re-sharpens on next load automatically.

## Acceptance

- [ ] Covers render with 10% unsharp-mask sharpening applied exactly
      once at decode; a second `LoadImage` of the same path returns the
      already-sharpened cached image (no double sharpen, no per-frame
      cost).
- [ ] Non-cover images (icons etc.) are pixel-identical with the hook
      installed.
- [ ] Large covers (>500 KB, async decode path) are sharpened too.
- [ ] Flat/placeholder images unchanged (sharpen of a flat field is a
      no-op); alpha channel preserved.
- [ ] vendor-patches.md gains the v0.18 entry and a guard test fails if
      the patch is lost (e.g. after `go mod vendor`).
- [ ] `go test ./...`, `go vet`, gofmt clean; docs swept (log.md).

## Outcome

Shipped as specified. Investigation confirmed covers are never resized
on disk (only center-cropped to 2:3 by `normalizeCover`); shirei scales
them live at render from the once-per-path decoded RGBA, so the
sharpening went in at the decode seam:

- Vendor patch v0.18 adds `SetImageDecodeHook`/`applyImageDecodeHook`
  to `LoadImage`, applied at both decode sites (sync and the >500 KB
  async path). Guarded by `TestVendorImageDecodeHookPatch`; documented
  in docs/vendor-patches.md.
- `internal/gui/sharpen.go`: `sharpenRGBA` (10% unsharp mask, 3x3
  edge-replicated box blur, RGB only, fresh buffer) and
  `installCoverSharpen`, wired from `newModel` via the new
  `covers.Covers.CacheDir` / `ui.Session.CoversCacheDir` accessors.
  Only cover-cache paths are sharpened; disk bytes stay pristine and a
  refetch re-sharpens on next load automatically.
- Six new tests, all green: flat no-op, exact edge math (100/160 →
  98/162), alpha + source preservation, cover-only routing with a
  second-load cache hit, the async large-file path, and the vendor
  guard. Red witnessed as compile red (`tmp/test-red-038.log`); green
  run `tmp/test-green-038.log` plus uncached `-count=1` reruns.
- Caveat: the full-suite green run shows one failure in the foreign
  session's in-flight red `caret_churn_test.go` (untracked,
  image-unrelated); `internal/gui` is green with that file set aside.
  Its comment cites "issue 038" — a numbering collision; this issue's
  doc (created 15:36) predates their file (15:46), so theirs should
  renumber.

Commit: <hash>

