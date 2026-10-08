---
title: Prefix picker menus cap at 20 rows and scroll
description: The umu prefix picker popups (GUI settings + detail, TUI modal) show at most 20 rows; longer lists scroll by wheel and follow the keyboard highlight.
status: done
---

# 056 — prefix picker menus cap at 20 rows and scroll

## What and why

Steam libraries alone can contribute dozens of compatdata prefixes, and
the prefix picker popups grew unbounded (off-screen). Both prefix menus
— the settings global default and the detail panel per-game pin — now
render at most 20 rows; the rest scrolls.

## Acceptance

- [x] GUI popups render a 20-row window; the wheel scrolls it (consumed,
  so nothing behind scrolls) and the arrow-key highlight pulls the
  window along; clicking windowed rows still picks.
- [x] TUI prefix/restore modals window at 20 rows around the selection
  (was 8).
- [x] `go vet` and `go test ./...` stay green.

## Outcome

GUI: `dropdownPopupCapped` windows the row list (`dropdownState.top`)
inside the shared popup — body renders `rows[start:end]`, the wheel
moves the window while the popup is hovered, and the window clamps the
highlight into view. A Viewport-based scroll was tried first and
abandoned: Viewport sizes extrinsically, which collapses inside the
intrinsically-sized popup panel, and clipped rows report garbage rects
for the click seams. Windowed rendering keeps the panel intrinsic, so
rects, clicks, and dismissal behave exactly like an uncapped menu.
Knob found by test: the trigger's `dropdownMenuKeys` needs the TOTAL
row count, not the rendered window's, or the highlight wraps early.
TUI: `pickerWindowRows` 8 → 20 (shared by the restore modal).
Commit: THIS_COMMIT.
