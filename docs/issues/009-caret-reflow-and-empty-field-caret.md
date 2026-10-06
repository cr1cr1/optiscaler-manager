---
title: Text caret no longer reflows letters; empty focused fields show a caret
description: Fix the in-flow caret sliding letters around on caret moves and blink, and restore the caret in empty focused fields (the add-fork inputs looked dead).
issue: 9
status: done
---

# 9 — Text caret no longer reflows letters; empty focused fields show a caret

## What and why

Two caret bugs in the shared GUI text field (`themedInputState`):

1. **Moving the caret moves letters around.** The caret was an in-flow
   `FixSize(2, 13)` container sandwiched between two separately shaped
   Labels (`r[:cursor]` + `r[cursor:]`). Every caret move changed the
   left label's width, so shirei's layout easing slid the tail label to
   its new spot; every blink toggle inserted/removed the 2px slot,
   shifting the whole tail ±2px; and split shaping changed kerning at
   the boundary. The buffer was never corrupted — pure reflow.
   Fix: render the buffer as ONE Label (shaped once, so letter positions
   are identical for every caret position) and paint the caret as a
   float child of the text row (`Float`, out of layout flow) at the
   shaped advance of the cursor. Blink only toggles whether the bar
   paints; nothing in layout changes.
2. **Empty focused fields show no caret at all.** Issue 8 removed the
   caret from the empty branch so the hint could stay without resizing
   the field; the add-fork slug/glob inputs start empty, so focusing
   them showed a hint and no caret — they looked dead. Issue 8's width
   stability is preserved because the floating bar takes no layout
   space: the empty branch now renders hint + caret at position 0.
   This supersedes issue 8's "focused-empty caret is gone by design".

Also: arrow/Home/End moves now wake the blink (caret visible right
after a move, per standard editing feel).

## Acceptance

- [x] Empty focused field paints a caret
  (`TestEditEmptyFieldShowsCaretWhenFocused`: red — no caret painted).
- [x] Blink phase never moves the text ink
  (`TestEditCaretBlinkDoesNotMoveText`: red — ink edge 51.9 vs 49.9, the
  exact 2px caret slot).
- [x] Caret moves wake the blink (`TestEditCaretMoveWakesBlink`: red —
  caret stayed hidden after an arrow move).
- [x] Caret x matches the shaped advance of the cursor
  (`TestEditCaretXTracksCursor`).
- [x] Issue 8's width/geometry stability tests still pass unchanged.
- [x] `go test ./...` green (29 packages), `go vet`/`gofmt` clean.

## Outcome

Seams added first (`caretVisible`/`caretX`/`inkRight` on `editState`,
mirroring the existing `boxRect`/`textRect` pattern) to witness all
three behavioral reds on the old code. The fix renders the buffer as a
single Label plus `caretFloat`: a 2px bar floating in the text row at
`advanceUpTo(text, cursor)`, `NoAnimate` so shirei's relativeOrigin
easing can't glide it, bar height = row height so no cross-axis math.

Two shirei behaviors discovered and encoded in comments/tests:

- A float nested in a container without in-flow children is never
  sized by the layout pass and paints nothing — the first iteration's
  zero-width anchor + floating bar rendered an empty rect (proven by a
  three-case probe), so the bar floats directly in the text row.
- `RenderToImage`'s settle loop can exceed the 500ms blink interval in
  wall time, and system fonts arrive on a background scan, so
  font-dependent tests call `WaitForSystemFontScan()` (without it,
  isolated `-run` executions shape zero-width text and the geometry
  assertions pass vacuously).

Visual verification via rendered PNGs: mid-text caret sits exactly at
the split with letters undisturbed; the empty focused field shows the
caret over the hint's left edge.
