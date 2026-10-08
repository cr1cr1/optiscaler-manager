---
title: Log panel behind a status-bar Log button
description: The bottom status bar stays one line; a Log button on its left slides up a readonly, selectable log panel showing the same console output (status history incl. umu stderr tails).
status: done
---

# 052 — log panel behind a status-bar `Log` button

## What and why

Today every status update — including launch failures whose error text
carries umu's multi-line stderr tail (issue 048) — is rendered inline in
the bottom status bar. A multi-line `StatusLine` makes the whole bar
grow vertically and squeezes the game grid.

Instead, the status bar stays a single line (long/multiline text is
truncated), and a small `Log` button on the bar's left edge toggles a
log panel that slides up above the bar, showing the same console output
in a readonly, selectable view.

## Design

- **Source of truth:** `ui.Session` keeps an append-only ring of status
  lines (capped, e.g. 512 lines). Every `setStatus` call appends its
  text (multi-line text split into lines), so the panel shows exactly
  what the status bar would have shown, including stderr tails. New
  accessor `LogLines()` returns a copy.
- **GUI sync:** the model owns a shirei `widgets.TextRing`; each frame
  it appends the session lines it has not seen yet (rebuilds on ring
  eviction mismatch).
- **Panel:** mounts between the content view and the status bar, full
  content width, height clamped (≈35% of window height, min/max bound).
  Header row: "Log" title + Close button. Body: `widgets.LogView`
  (readonly by construction, drag-select across lines, Ctrl/Cmd+C copy,
  bottom-pinned auto-scroll until the user scrolls up).
- **Slide-up:** the panel height tweens 0 → target on open and back on
  close via the frame pulse (issue 040 mechanism), or shirei layout
  animation if it covers mount/unmount; verified visually.
- **Dismiss:** `Log` button toggles; Esc closes the panel first (before
  the detail panel) via `handleGlobalKeys`; Close button in the header.
- **Status bar:** `StatusLine` rendered on one line (first line only,
  ellipsis-truncated); the bar's height never changes with content.
- **TUI:** unchanged (the ring is additive; the TUI does not render it).

## Acceptance

- [x] After a launch failure with a multi-line stderr tail, the status
  bar remains one line tall (`TestStatusBarHeightIgnoresMultilineStatus`).
- [x] Clicking the `Log` button slides the panel up; it lists the
  status history including the full stderr tail
  (`TestLogButtonTogglesPanel`, `TestLogPanelShowsSessionLines`).
- [x] Log text is selectable with the mouse and copyable (Ctrl+C);
  editing is impossible (shirei `widgets.LogView`: no input widget).
- [x] Panel auto-scrolls to the newest line while pinned; scrolling up
  unpins; scrolling back to bottom re-pins (shirei LogView behavior).
- [x] Esc or a second click on `Log` closes the panel; reopening keeps
  the history (`TestEscClosesLogPanelFirst`; the ring lives in the
  session, not the panel).
- [x] Ring is capped: >512 status lines drop the oldest without
  crashing the panel sync (`TestLogRingCapsOldest`,
  `TestSyncLogLinesAppendsAndRebuilds`).
- [x] `go test ./...` and `go vet ./...` pass.

## Outcome

Done in commit cab2d31. Status bar is single-line always (`oneLineStatus` collapses
multi-line text to its first line + " …"); `ui.Session` keeps a capped
(512-line) append-only status ring exposed via `LogLines()`; the GUI
mirrors it into a `widgets.TextRing` (`syncLogLines`, full rebuild on
eviction drift) and slides a readonly/selectable `widgets.LogView` panel
up behind a new `Log` button on the bar's left edge (160ms ease-out
height tween, `NoAnimate` shell). Esc closes the log panel before the
detail panel. Red witnessed as build failures on the new seams, then
green; two unrelated flakes during verification (`TestWarmBootResolvesDefault`
under `-count=1` load, `TestFindPrefixes_BottlesCustomPath` from a
parallel session's half-edited `internal/umu`) both passed on re-run.
