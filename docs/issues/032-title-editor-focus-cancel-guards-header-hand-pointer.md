---
title: Title editor focus/cancel/no-op guards; header button hand pointer
description: Set title moves focus into the input, hiding the details pane cancels the edit, unchanged titles are never written, and the header buttons show the hand pointer.
---

# 032 — Title editor focus/cancel/no-op guards; header button hand pointer

## What and why

Follow-up to issues 029/031, three title-editor refinements and one
cursor fix:

1. **Focus follows the editor**: pressing Set title arms a deferred
   focus grab (`titleFocusPending`, the `cardFocusPending` idiom) — the
   input does not exist until the header renders it next frame, so the
   grab fires there with the freshly captured `titleInputID`.
2. **Hiding the pane cancels the edit**: an open editor whose panel
   disappears (Close, Esc at panel level, selecting nothing) or whose
   game changes is cancelled via `cancelTitleEdit` — in rootView's
   panel-absent branch AND in `detailPanel` when the selected row is
   gone or different. An abandoned buffer can never apply later.
3. **No-op writes skipped**: `applyTitleEdit` compares the trimmed
   buffer against `titleEditOrig` (the title as the user saw it when
   the editor opened) and skips `SetTitleOverride` when unchanged — a
   no-op commit would still persist settings and kick a cover
   re-resolution for nothing.
4. **Hand pointer**: `panelHeaderButton` (Set title, Apply, Cancel,
   Close) gained `PointerHand`, matching `focusableButton`.

## Acceptance

- [x] Clicking Set title opens the editor AND the input holds focus the
      next frame (real mouse click/release frames).
- [x] `Select("")` with an open editor cancels it (dir/buf/orig/state
      all cleared); the abandoned title is not applied.
- [x] Applying an untouched buffer writes nothing (`TitleOverrides`
      stays empty); changed titles still write (issue 028's test holds).
- [x] Hovering Set title and Close picks `CursorShapePointer`.
- [x] Docs updated: `docs/scope.md`, `docs/log.md`.

## Outcome

Done. TDD red witnessed on all four behaviors (no focus move, edit
survived panel hide, unchanged title written, default cursor). One
test initially still failed after the `detailPanel` guard: the panel is
never CALLED when nothing is selected, so the cancel also lives in
rootView's panel-absent branch. Full `go test ./...` exit 0 (31
packages), `go vet`/`gofmt` clean. Commit: fcdc4f8.
