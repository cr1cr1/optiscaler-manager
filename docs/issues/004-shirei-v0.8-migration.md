---
title: Migrate vendored shirei to v0.8.0 and reapply the vendor patches
description: Finish the uncommitted shirei v0.6.10 → v0.8.0 dependency bump; reapply the nine optiscaler-manager vendor patches onto v0.8.0 and migrate internal/gui off the removed ButtonAccent global onto the color-scheme API.
issue: 4
status: done
---

# 4 — Migrate vendored shirei to v0.8.0 and reapply the vendor patches

## What and why

The workspace carried an incomplete dependency bump (shirei v0.6.10 →
v0.8.0, plus teatest/brotli/lz4 refreshes and a full `go mod vendor`):
`internal/gui` no longer compiled because the re-vendor wiped the
optiscaler-manager patch set (`PointerHand`/`TextEntry` from the v0.17
cursor patch, `ImageFill` from v0.14, and the Win32/Wayland key-repeat,
CSD, scroll, animation, and headless-reset patches), and shirei v0.8.0
removed the `widgets.ButtonAccent` global the GUI theme set. This issue
finishes the bump: every patch is reapplied onto v0.8.0 (verbatim copies
where upstream did not touch the file, hunk-level adaptation where it
did), and the GUI theme moves to the v0.8.0 color-scheme API with a
`Buttons.Default` paint that reproduces the old computed ButtonAccent
look exactly (TopBoost 8, ElevationDrop 16, same hover/press lightness
deltas).

## Acceptance

- [x] `go build ./...` and `go vet ./...` clean.
- [x] `TestVendorCSDPatchPresent` passes — every v0.5/v0.8/v0.9/v0.10/
      v0.11/v0.13/v0.14/v0.16/v0.17 marker present in the v0.8.0 vendor
      tree, plus the v0.12/v0.15 upstream-mechanism guards.
- [x] The cursor behavior tests (`internal/gui/cursor_test.go`) pass.
- [x] `go test ./...` fully green.
- [x] `docs/vendor-patches.md` records the v0.8.0 reapplication.

## Outcome

- Five patched files upstream did not touch between v0.6.10 and v0.8.0
  were copied verbatim (`images.go`, `waylandkeyboard_linux.go`,
  `waylandcursor_linux.go`, `waylandinput_linux.go`); six were adapted
  hunk by hunk (`shirei.go`, `attrs.go`, `softrender.go`, `renderpng.go`,
  `waylanddecor_linux.go`, `waylandbackend_linux.go`,
  `win32backend_windows.go`). The v0.8.0 `wmKillfocus` gained an
  accessibility `refreshAccess()` line; the cancel-repeat hook anchors
  after it.
- `internal/gui/theme.go` replaces `widgets.ButtonAccent = accent` with a
  `widgets.DarkColorScheme()`-derived `CurrentColorScheme`: the default
  button role's `ButtonStyle` recomputes the exact pre-v0.8.0 paint from
  the app accent, and `FocusRing` uses the app's focusBorder.
  `DefaultBackground`/`DefaultAccent` still exist upstream and stay.
- No shirei patch turned out to be superseded by v0.8.0: its new
  `ImageViewAt` still emits `ImageScale: true` surfaces, so the v0.14
  stretch patch remains necessary for gap-free cover art.
- Full suite green (including the headless GUI widget tests and the
  vendor guard) on the final tree; commit hash recorded in docs/log.md.
