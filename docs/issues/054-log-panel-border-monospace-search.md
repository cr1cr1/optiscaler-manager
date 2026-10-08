---
title: Log panel polish — border, monospace, search with hit highlight
description: The slide-up log panel (052) gains a thin border, monospace log text, and a search field with a lupe button (left of the right-most Close) that jumps to the matching line and highlights the term.
status: done
---

# 054 — log panel: border, monospace, search with hit highlight

## What and why

Follow-up to issue 052: the log panel blends into the content above it,
renders log text proportionally, and offers no way to find a line in a
long history. Add a thin border around the panel, switch the log text to
shirei's default monospace stack, and add a search field with a
lupe-icon button — both left of the `Close` button, which stays
right-most — that jumps to the matching line and highlights the term.

## Design

- **Border**: `BorderWidth(1), BorderColorVec(border)` on the panel
  shell (theme's standard border color).
- **Monospace**: `TextStyleAttrs.SetFontFamilies(Monospace...)` on the
  `LogView` text style.
- **Header layout**: `[Log title] [Filler] [search field] [lupe button]
  [Close]` — Close remains the right-most control.
- **Search**: the lupe button (or Enter in the field) runs
  `logSearchNext`: case-insensitive substring over the ring; repeated
  submits with an unchanged query advance to the next match (wraps);
  jump via `VirtualListScrollIntoView(listKey, ring.LineID(idx))`.
  Opening the panel focuses the search field.
- **Highlight**: vendor patch **v0.22** adds
  `widgets.LogViewSearch(ring, attrs, listKey, probe, query, hitColor)`,
  threading the query into the item view where every case-insensitive
  match range becomes a `StyleSpan` with a `Background` highlight
  (overlay semantics keep the base font/color). The live buffer is the
  query, so highlighting follows typing; the jump happens on submit.
- **Pin interaction**: an upward search jump unpins the tail-follow
  (LogView's own rule: scrollY decrease unpins); new lines arriving
  while pinned resume tail-follow, as before.

## Acceptance

- [x] Panel has a visible 1px border on all sides
  (`TestLogPanelBorderPainted`, PNG seam scan at `HeadlessScale`).
- [x] Log text renders in the monospace stack
  (`TextStyleAttrs.SetFontFamilies(Monospace...)`).
- [x] Header order: search field and lupe button left of `Close`;
  `Close` is the right-most control (`TestLogPanelHeaderOrderCloseRightmost`).
- [x] Enter or lupe click with a term scrolls the first/next match into
  view (`TestLogSearchScrollsMatchIntoView` via `LogViewProbe`);
  repeating cycles through matches and wraps (`TestLogSearchFindAdvancesAndWraps`).
- [x] All visible occurrences of the term are highlighted (vendor patch
  v0.22 `LogViewSearch`: `Background`-only `StyleSpan` per match).
- [x] Empty query clears the hit and highlights nothing
  (`logSearchFind` returns -1 on empty).
- [x] Vendor patch v0.22 marker present and guarded
  (`TestVendorCSDPatchPresent`); `docs/vendor-patches.md` updated.
- [x] `go test ./...` and `go vet ./...` pass (32/32 in an isolated
  worktree with only this issue's files; the shared tree was mid-edit
  by the parallel session during verification).

## Outcome

Done. Panel shell gained `BorderWidth(1)`+`border`; log text uses
shirei's `Monospace` stack; the header is `[Log] [Filler] [search]
[lupe] [Close]` with focus-on-open in the search field (Esc layering
mirrors the title editor: first Esc clears/blurs the field, second
closes the panel — pinned by `TestEscClosesLogPanelFirst`). Search:
`logSearchFind`/`logSearchNext` over the ring, wrap-around advance on
repeat submits, jump via `VirtualListScrollIntoView(logListKey,
ring.LineID(hit))`; highlight follows the live buffer through vendor
patch v0.22 (`LogViewSearch`). Commit hash recorded in a follow-up
commit.
