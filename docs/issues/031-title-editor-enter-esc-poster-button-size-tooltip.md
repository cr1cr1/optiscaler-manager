---
title: Title editor Enter/Esc keys; larger poster button with tooltip
description: Enter applies and Esc cancels in the detail panel's title editor input; the poster overlay button is 30% larger and shows a "Set Poster" tooltip.
---

# 031 — Title editor Enter/Esc keys; larger poster button with tooltip

## What and why

Follow-up to issue 029:

1. **Keyboard flow in the title editor**: pressing Enter in the input
   applies (same as the Apply button), pressing Esc cancels (same as
   Cancel — and no longer closes the panel). The shared `editKeys`
   consumes both keys by default (Enter: leak guard; Esc: clear+blur),
   so a post-render key check never sees them — the fix is per-field
   hooks: `editState` gained optional `onEnter`/`onEscape`, invoked from
   the Enter/Esc cases when set. The title editor owns its state
   (`m.titleEditState`, created by `startTitleEdit` with the two hooks,
   dropped by the shared `closeTitleEdit`). Search and settings fields
   keep the defaults.
2. **Poster overlay button**: `TextSize: ButtonDefaultSize * 1.3` (30%
   larger — TextSize scales icon and padding) and a hover tooltip via
   the newly generalized `hoverTip(text)` (issue 020's pill tooltip
   machinery extracted from `pillHoverTip`, same 500ms resting
   debounce), showing "Set Poster".

## Acceptance

- [x] Enter in the focused title input applies (row renamed through the
      session); Esc cancels (row untouched) and the panel stays open.
- [x] Poster button height ≈1.3× the default button's (asserted 1.15–1.5
      to absorb font shaping).
- [x] Hovering the poster button shows the "Set Poster" tooltip after
      the debounce (3 warm-up frames for font settling, then backdated
      clock).
- [x] Search/settings field keys unchanged (`TestEdit*` suite holds).
- [x] Docs updated: `docs/scope.md`, `docs/log.md`.

## Outcome

Done. TDD red witnessed: new tests failed to build (`titleInputID`
undefined), then failed at runtime — the debug run proved focus landed
but `editKeys` ate both keys (Esc even cleared the buffer), which
motivated the hook design over a post-render switch. Full
`go test ./...` exit 0 (30 packages), `go vet`/`gofmt` clean.
Numbering note: planned as 030, renumbered to 031 — the concurrent
session claimed 030 (title cleanup / cover search) first.
Commit: 14b04c7.
