---
title: Tooltip debounce requests frames so a resting cursor reaches 500ms
description: The 500ms pill-tooltip debounce (issue 020) only elapsed when new frames rendered, but the backend renders on input alone — a resting cursor never saw the tooltip until the mouse moved. The pending debounce now requests the next frame each frame.
---

# 021 — Tooltip debounce requests frames so a resting cursor reaches 500ms

## What and why

User report on issue 020: "tooltip display is now broken, have to move the
mouse over the pill to appear. Just wanted a delay/debounce of 500ms if the
cursor rests over a pill."

Root cause: the debounce clock is only *evaluated* when a frame renders,
and the shirei backend renders on input alone (`FrameRequested` gates the
idle loop). A cursor resting on a pill produces no input events, hence no
frames, hence the 500ms never elapsed — the tooltip popped on the next
mouse movement, exactly the reported symptom.

Fix: while a tooltip is pending but not yet due, `pillTipOverlay` calls
shirei's `RequestNextFrame()` — the wake mechanism built for "animations
and state that settles over several frames" — so frames keep coming until
the debounce elapses and the overlay renders. Once shown, no further
requests: the idle loop goes quiet again (a mouse move away generates its
own input frame, which clears the tooltip).

## Acceptance

- [x] While the debounce is pending, each frame requests its successor
      (`FrameRequested()` true with no new input).
- [x] Once the tooltip is shown, no further frames are requested (no idle
      spin).
- [x] Tooltip still never renders before 500ms, still clears on mouse-away.
- [x] Full `go test ./...` green; `go vet`/`gofmt` clean.

## Outcome

Done. One-line change in `pillTipOverlay` (internal/gui/tooltip.go) plus
`TestPillTooltipRequestsFramesWhileDebouncing`
(internal/gui/tooltip_frame_test.go): hover with a parked mouse asserts a
frame is requested while pending, then — with `pillTip.since` backdated —
the tooltip shows and `FrameRequested()` is false again. TDD red witnessed
("debounce pending but no next frame requested"), then green. Full suite
(29 packages) exit 0. Commit TBD.
