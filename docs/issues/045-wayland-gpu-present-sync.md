---
title: Wayland GPU presents must wait for render completion (glFinish)
description: With the GPU path re-enabled by issue 043, the Wayland backend presented dmabufs while the GPU could still be drawing them (wait=false is a bare glFlush on Linux GLES; no completion fence exists), producing severe clipping under cursor-driven repaint bursts. The backend now renders with wait=true.
---

# 045 — Wayland GPU presents must wait for render completion (glFinish)

## What and why

User report after the issue-043 modifier-retry fix re-enabled the GPU
path: severe clipping, especially while moving the cursor over the app.

**Root cause** (vendored shirei inspection): `drawFrame` renders with
`gpurender.Render(..., wait=false)` and immediately attaches + commits
the buffer. On the Linux GLES backend `wait=false` ends in `glFlush()`
only — command submission, not completion. No fence is handed to the
compositor, and the async-completion hook (`SetCompleteFunc`) is wired
only by the Cocoa backend; on GLES it fires immediately after the flush
anyway, so it would not synchronize anything. The compositor therefore
samples the dmabuf mid-render whenever the frame rate outruns the GPU —
exactly the cursor-movement case (hover drives 60-150 repaints/s per
the frame trace). Partially drawn presents read as clipping. The issue
was invisible while the probe failure (043) kept the app on the
software renderer, whose CPU raster completes synchronously.

**Fix** (vendor patch v0.20): `wait=true` at the Wayland `Render` call —
glFinish before attach/commit. Upstream already reserves that mode for
"tests / Wayland M1"; NVIDIA GBM needs it too. Cost is sub-millisecond
per frame at this app's quad count.

## Acceptance

- [x] Guard test: `TestVendorCSDPatchPresent` checks the v0.20 marker
      (hardware behavior verified by the user — sandboxed shells have no
      /dev/dri).
- [x] `go build ./...`, `go vet ./...`, gofmt clean; full `go test ./...`
      green (except the foreign session's hardware-gated tmp/gpuprobe).
- [x] Docs updated: `docs/vendor-patches.md` (v0.20), `docs/log.md`.

## Outcome

Patch applied; user verification: clipping gone on next run. If GPU
frame pacing ever proves heavy, `SHIREI_GPU=0` is the escape hatch and a
fence-based async present (EGL_ANDROID_native_fence or explicit sync) is
the upgrade path over blocking glFinish. Commit: TBD.
