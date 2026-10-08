---
title: Toast overlay clipped at the window bottom
description: Launch-failure toasts carry the multi-line stderr tail, making the card taller than the 40px slot so the stack overflows the window bottom; raise the stack above the status bar and clamp toast text to one line.
status: done
---

# 058 — toast overlay clipped at the window bottom

## What and why

After a failed launch the bottom-right toast shows
`"Launch failed: <multi-line stderr tail>"`. `toastOverlay` reserves a
constant 40px per card, but the card auto-sizes with the wrapped text,
so the stack grows past the window bottom and the lowest toast is cut
off. The toast must sit above the status bar and never be clipped.

## Design

- Toast text renders single-line (`oneLineStatus` — the full text lives
  in the log panel since 052, so nothing is lost).
- The stack's bottom margin rises from a fixed `sp24` to the status
  bar's top edge (its captured height + spacer), so toasts never
  overlap or cross the window's bottom edge.

## Acceptance

- [x] With a multi-line failure toast, every visible toast card is
  one line tall and fully inside the window above the status bar
  (`TestToastRaisedAboveStatusBarAndSingleLine`).
- [x] `go test ./...` and `go vet ./...` pass (32/32).

## Outcome

Done in commit c49f89f. Toast cards render `oneLineStatus(t.Text)` (full text remains in
the log panel) and the stack bottom clears the status bar via its
captured height + spacer. Commit hash recorded in a follow-up commit.
