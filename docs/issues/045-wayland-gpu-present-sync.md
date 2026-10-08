---
title: Wayland GPU presents wait for render completion
description: After issue 043 re-enabled the GPU path, hover repaints presented partially rendered frames (severe clipping); the Wayland backend now waits for render completion before present.
---

# 045 — Wayland GPU presents wait for render completion

## What and why

User report (after the issue-043 fix re-enabled GPU rendering): the
"software rendering" message was gone, but there was severe clipping over
the entire app, worst while moving the cursor.

Root cause (vendor inspection): the Wayland backend's `drawFrame` calls
`gpurender.Render(..., wait=false)` and immediately attaches/commits the
dmabuf. On the Linux GLES path `wait=false` is a bare `glFlush` — command
submission, not completion — and no completion fence is plumbed to the
compositor; `SetCompleteFunc` is wired only by the Cocoa backend and
fires right after the flush on GLES anyway. The compositor therefore
sampled the buffer mid-render whenever cursor-driven hover repaints
(60–150/s in the frame trace) outran the GPU. The defect was invisible
while issue 043's probe failure forced software rendering, because the
CPU raster completes synchronously.

Fix: vendor patch v0.20 — pass `wait=true` (`glFinish` before
attach/commit) at the Wayland `Render` call. Upstream's own comment
reserves that mode for "tests / Wayland M1"; NVIDIA GBM needs it too.
The per-frame cost is sub-millisecond at this app's draw load.

## Acceptance

- [x] Wayland `drawFrame` presents only after render completion
  (vendor patch v0.20, marker comment in
  `waylandbackend_linux.go`).
- [x] `TestVendorCSDPatchPresent` guards the patch against a
  `go mod vendor` wipe.
- [x] `docs/vendor-patches.md` documents v0.20 (what, why, how to
  reapply).
- [ ] User confirms the clipping is gone (pending at commit time;
  `SHIREI_GPU=0` remains the escape hatch).

## Outcome

One-line vendor patch (v0.20) plus guard test and docs; full suite green
(except the foreign session's sandbox-blocked `tmp/gpuprobe` hardware
probe). Deferred, documented: fence-based async present
(EGL_ANDROID_native_fence) is the upgrade path over blocking `glFinish`;
a runtime mid-session `Render` error still permanently disables the GPU
(`slot = nil`).

Commit: 6572445 (docs pin: f2c69d4).

<!-- Doc content restored 2026-10-09: the file was emptied by foreign
     session churn before its pin commit; reconstructed from docs/log.md
     and docs/vendor-patches.md (v0.20 section), which survived intact. -->
