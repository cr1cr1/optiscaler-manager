---
title: Card click on the last grid row must not scroll the view to the top
description: Clicking a card on the grid's partially-filled last row opened the detail panel but reset the card view's scroll to the top; the click path now arms the same deferred scroll-into-view restore the keyboard Enter path arms.
---

# 037 — Card click on the last grid row must not scroll the view to the top

## What and why

User report: clicking a card on the grid's last, not-fully-filled row
opens the details pane but scrolls the card view back to the top.
Navigating with arrows and pressing Enter does not.

**Root cause** (systematic debugging, confirmed by the repro test):
opening the detail panel re-nests the grid — shirei identities are
path-scoped, so the virtual list's node is recreated with scroll offset
0. Restoration relies on the deferred `scrollCursorPending` →
`VirtualListScrollIntoView(selIdx/cols)` mechanism. The keyboard Enter
path arms it unconditionally; the click path never did. The
identity-churn fallback (re-arm when the cursor card's node id changes)
only fires when the cursor card renders *inside the reset top window* —
a deep, last-row card never renders there, so nothing ever restored the
scroll and the view stayed at the top. Cards on early rows appeared to
work only because the churn fallback covered them.

**Fix**: the card press gesture now also sets `scrollCursorPending`
(grid.go), one line mirroring the Enter path — same timing, already
proven flicker-free by the keyboard path.

## Acceptance

- [x] Repro test `TestGridClickLastRowKeepsScroll` (14 games, partial
      last row): scroll the last row into view, click the last card,
      panel opens, and after settling the clicked card is still in the
      painted window. Red before the fix (card no longer rendered),
      green after.
- [x] No regression on the plain click-select path
      (`TestCardBodyClick_FiresSelect` and the grid suite stay green).
- [x] Docs updated: `docs/scope.md`, `docs/log.md`.

## Outcome

Done. TDD red witnessed (`tmp/issue37-red.txt`): clicked last-row card
absent from the painted window after the panel opened. One-line fix;
full `go test ./...` green for every package except the foreign
session's mid-TDD issue-035 rescan tests (`internal/tui` — their red
phase, untouched by this change). `go vet`/`gofmt` clean. Commit:
ed7eb66.

Deferred probe: the list view's row click has the same shape (Select
without arming a scroll restore); not reported and not reproducible
without a very long library — revisit if it ever surfaces.
