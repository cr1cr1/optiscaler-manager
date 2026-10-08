---
title: Time-driven widgets must ride a shared pulse, not per-frame requests
description: The focused-field caret blink and the busy spinner requested a frame on every frame, keeping the GUI's frame loop hot and re-presenting the whole canvas — visible as a subtle whole-window "refresh" flicker; both now signal a frame-scoped pulseWanted flag serviced by a 10 Hz pulse ticker, and session events get a proper frame-request bridge.
---

# 040 — Time-driven widgets must ride a shared pulse, not per-frame requests

## What and why

User report: from time to time a subtle artifact washes over the entire
GUI, as if the app is refreshing — not tied to any particular component.

**Root cause** (systematic debugging; mechanism confirmed by red tests,
user confirmed Wayland): the app is event-driven — shirei skips painting
unchanged frames, and the repo already enforces that discipline for
dropdown menus (dropdown_idle_test.go: "sustained churn would present as
flicker on wayland"). Two widgets violated it: `caretFloat` requested a
frame on **every** frame while any text field held focus (search box,
settings inputs, title editor), and `spinnerGlyph` did the same while
rendered. Each content flip (caret 500 ms, spinner 150 ms) then
re-presented the **whole canvas**: the Wayland backend damages the full
surface per present and the X11 backend re-renders and blits every
produced frame unconditionally. Hence a whole-window "refresh" that
comes and goes with focus/busy state and points at no component.

**Fix** (three parts):

1. `caretFloat`/`spinnerGlyph` no longer call `RequestNextFrame()`; they
   set the package-level `pulseWanted` atomic for the frame (cleared by
   `rootView` at frame start, mirroring the `pillTip.seen` re-arm).
2. `pulseFrames` (started by `Run`): a 100 ms ticker that requests a
   frame only while `pulseWanted` is set — 10 Hz instead of one frame
   per frame callback. Blink/spinner cadence logic is unchanged (each
   still evaluates its own clock at frame time).
3. `forwardSessionEvents` (started by `Run`): each session event
   requests a frame. Previously the spinner's churn accidentally doubled
   as the event pump for scan progress/toasts; removing it without this
   bridge would have frozen background repaints until the next input.
   Safe to share the channel with `drain()`: both consumers discard the
   event value and re-snapshot.

The pill-tooltip debounce keeps its own bounded `RequestNextFrame`
(it goes quiet after the 500 ms debounce by design).

## Acceptance

- [x] `TestFocusedEditFieldIdleSettles`: a focused edit field settles to
      a quiet loop (red before: every idle frame requested another).
- [x] `TestSpinnerIdleSettles`: same for the busy spinner (red before).
- [x] `TestTimeDrivenWidgetsSignalPulse`: the focused field and the
      spinner set `pulseWanted`; a blurred field does not (red before:
      seam did not exist).
- [x] Full `go test ./...` green; `go vet`/`gofmt` clean.
- [x] Docs updated: `docs/scope.md`, `docs/log.md`.

## Outcome

TDD red witnessed for all three tests before the fix. `internal/gui`
plus the whole suite green after. The visible confirmation (artifact
gone in the real compositor) is the user's call; if any residue
remains, `SHIREI_TRACE_FRAME=1` prints per-second frame-wake reasons to
stderr for the next round. Commit: TBD.
