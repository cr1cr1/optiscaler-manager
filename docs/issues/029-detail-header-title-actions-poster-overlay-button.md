---
title: Detail panel header hosts title actions; poster hosts Set-poster overlay
description: The Set-title button moves into the detail panel header next to the title (left of Close) with wrap-safe layout, and Set poster becomes an icon-only button on the poster's own top-left corner.
---

# 029 — Detail panel header hosts title actions; poster hosts Set-poster overlay

## What and why

Follow-up to issue 028's manual identification fixes: the Set title and
Set poster buttons lived in the bottom action list, far from what they
edit. Now:

1. **Set title sits in the header** — `[title] [Set title] [Close]`. The
   title container caps at the content width (MaxSize cascade soft-wraps
   the label) and the header row itself wraps, so a long title can never
   push the buttons out of the visible panel. The title editor (input +
   Apply/Cancel) opens in place, replacing the label and button. The
   header buttons use the new `panelHeaderButton` helper (generalized
   from `panelCloseButton`): the first one rendered captures
   `m.panelFirstID`, preserving the Tab-continuation jump target.
2. **Set poster is an icon-only button** (SymImage, no label) floating
   on the poster's own top-left corner with the default margin
   (`Float(sp8, sp8)` relative to the poster box, so it scrolls with
   it). Placeholder covers get it too — they need it most. "Reset
   poster" stays in the action list.

## Acceptance

- [x] Header order title → Set title → Close, all in the band above the
      poster (seams: `panelTitleRect`, `setTitleRect`, `closeBtnRect`,
      `posterRect`).
- [x] A long title wraps (label height grows) at minimum panel width and
      no header element crosses the panel's right edge.
- [x] Poster overlay button: icon-only (roughly square), inset ~sp8 from
      the poster's top-left (seam: `posterBtnRect`).
- [x] While editing, label and Set-title button are replaced by the
      in-header editor (Apply captured via `titleApplyRect`); Close
      stays.
- [x] Docs updated: `docs/scope.md`, `docs/log.md`.

## Outcome

Done. TDD red witnessed as build failure on the new seams, then green;
one test needed the title changed at session level (`drain()`
re-snapshots `m.state` every frame) and trims trailing spaces. Full
`go test ./...` exit 0 (30 packages), `go vet`/`gofmt` clean.
`panelFirstID` semantics preserved: now the header's Set-title button
(Apply while editing, Close without a session) — paneltab tests hold.
Deferred: Shift+Tab on the title INPUT itself (open editor) does the
default reverse walk, not the card continuation — the continuation seam
targets the first panelHeaderButton. Commit: see git log.
