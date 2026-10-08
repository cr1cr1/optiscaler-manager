---
title: GPU probe retries across BO modifiers instead of falling back to software permanently
description: AllocTarget allocated a LINEAR gbm buffer (preferred for scanout) that NVIDIA's driver cannot render to; the single-shot FBO-completeness check failed with "framebuffer incomplete" and the Wayland backend ran software-rendered for the whole session. The probe now tries every BO strategy until one imports as a complete framebuffer.
---

# 043 — GPU probe retries across BO modifiers instead of falling back to software permanently

## What and why

The frame trace captured from the user's session opened with
`gpurender: fallback to software: framebuffer incomplete` — the GPU
probe fails at startup and the app renders on the CPU for the entire
session (full-canvas software raster per present: slower, subtler
differences vs the GPU path, and the likely substrate of the "whole
canvas refresh" artifact's visibility).

**Root cause** (code inspection; machine is NVIDIA RTX 5080/4060,
proprietary 615.71.09, Wayland): `AllocTarget` in vendored shirei's
GLES/dmabuf path allocates one gbm buffer object — preferring a LINEAR
modifier when advertised (correct for scanout) — then imports it as an
EGL image/texture/FBO **single-shot**. NVIDIA's GBM allocates LINEAR
fine but rejects it as a render target, so `glCheckFramebufferStatus`
fails and the whole GPU path is abandoned even though the next
allocation strategy (implicit/block-linear modifier) would succeed.
The issue-041 audit separately confirmed the GPU path worked on this
machine under shirei v0.6.x, i.e. this is a robustness regression of
the dmabuf probe, not an unsupported host.

**Fix** (vendor patch v0.19): `AllocTarget` iterates the BO allocation
strategies in the original preference order (`glesBOStrategies`, split
out of `glesCreateBO`) and keeps the first whose import yields a
complete framebuffer; the import body (`importBO`) keeps upstream's
per-failure resource cleanup verbatim. Total failure still falls back
to software exactly as before.

## Acceptance

- [x] Guard test: `TestVendorCSDPatchPresent` checks the v0.19 marker,
      `glesBOStrategies`, and `importBO` (vendor-code convention — GPU
      behavior needs the physical driver; verified by the user).
- [x] `go build ./...`, `go vet ./...`, gofmt clean; full `go test ./...`
      green.
- [x] Docs updated: `docs/vendor-patches.md` (v0.19), `docs/log.md`.

## Outcome

Patch applied; awaiting user verification: on the next launch the
`gpurender: fallback to software` stderr line should be gone (the
backend logs the active GPU via perfLog). Commit: TBD.

Deferred: a *runtime* `Render` error mid-session still disables the GPU
permanently (`slot = nil` in drawFrame); not observed — revisit if the
trace shows it.
