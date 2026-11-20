---
title: Fix the build after the shirei v0.9.1 upgrade without vendor patches
description: The dependency upgrade wiped the vendored shirei patches; the app now builds against the upstream API and carries no vendor patches.
---

# 084 — Fix the build after the shirei v0.9.1 upgrade without vendor patches

## What and why

The `go.mod` upgrade (shirei v0.8.0 → v0.9.1, kong v1.17.0, and others)
regenerated `vendor/` and dropped every local shirei patch, so
`internal/gui` no longer compiled (`PointerHand`, `ImageFill`,
`LogViewSearchExt` undefined). Per the user's instruction, no patch is
reapplied to `vendor/`: the app code is fixed against the upstream
v0.9.1 API instead.

## Acceptance

- [x] `go vet ./...` and `go test ./...` pass; `vendor/` is byte-identical
  to the pristine v0.9.1 module for every vendored shirei file.
- [x] Dropped with the patches: the hand cursor over clickables, the log
  search-hit highlight, and the colored log game-title prefix (the prefix
  text stays). Cover art uses upstream `ImageViewAt` in the same box.
- [x] `docs/vendor-patches.md` records that no patches are carried.

## Outcome

Done. `internal/gui` builds against upstream shirei v0.9.1 with no
vendor patches: `PointerHand`/`TextEntry` attrs removed, cover art via
`ImageViewAt`, log panel via `LogViewExt`. Tests that pinned vendor
behavior were deleted (`cursor_test.go`, `csd_test.go`,
`loadimage_test.go`, the v0.24 list/text tests); the settings focus
tests were fixed at the app seam — `headlessFrames` now sweeps the
identity tree with empty frames, replacing the dropped v0.16 reset.
Also lost with the patches (recorded in `docs/vendor-patches.md`):
scroll speedup, no-animation snap, CSD-off default, Win32 key repeat,
GPU modifier retry, and the LoadImage missing-file recovery. Wayland
key repeat and Shift+Tab are upstream now.
