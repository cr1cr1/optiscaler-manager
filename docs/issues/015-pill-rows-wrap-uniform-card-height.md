---
title: Pills wrap in cards and detail pane; uniform card height
description: Long or numerous pills no longer overflow the card/detail pane — every pill row wraps, and all grid cards share the worst card's wrapped height so nothing clips.
---

# 015 — Pills wrap in cards and detail pane; uniform card height

## What and why

With several pills on one row — long fork distribution names (issue 013
made them longer: `✦ OptiScaler-DLSSNR-PreSR-Multipass 0.8.92`), component
pills, Proton — the single-line pill rows overflowed the visible area:
cards are `FixSize` + `Clip`, so pills past the inner width were cut off,
and the detail pane let them run past its edge.

Every pill row now wraps (shirei's `Wrap` attr): the card's badge row,
version pill row, and tech badge row, plus the detail panel's status and
version pill rows. The list view's fixed-height rows are untouched.

Wrapping makes one card's content taller, but card heights must stay
uniform: `cardContentHLines` sizes cards from per-row wrapped line counts
(badge / version / tech), and `gridView` takes the maximum of each across
ALL visible rows — so one card with a wrapped pill list grows every card
to the same height instead of clipping itself. Line counts come from
`wrapLineCount`, which mirrors shirei's greedy wrap packing (a pill wraps
only when it strictly exceeds the remaining line width), fed by width
estimates that mirror the rendered geometry: `Pad2(3, 6)` + 11 px label
for bare pills, the dropdown trigger's arrow allowance, and the DLSS
control's two segments. Estimates err on the wide side (taller cards are
safe; narrow ones would clip). With no wrapping anywhere the geometry is
byte-identical to before (one reserved line per pill row, as before).

## Acceptance

- [x] Pill rows wrap in game cards (badge, version, tech) and in the
      detail panel (status, version) — no pill is pushed outside the
      visible area.
- [x] All grid cards share one height; when any card's pills wrap, every
      card grows so the wrapped card never clips its pill row.
- [x] `wrapLineCount` matches shirei's greedy wrap (exact-fit does not
      wrap; first pill on a line never wraps).
- [x] `go vet`, `gofmt` clean; docs updated.

## Outcome

Done. `Wrap` on the five pill rows (internal/gui/grid.go gameCard ×3,
internal/gui/view.go detailPanel ×2); `wrapLineCount` +
`badgeRowWidths`/`versionPillWidths`/`componentPillW`/`techBadgeWidths` +
`cardPillLines` estimate wrapped lines; `cardContentHLines`/`pillBlockH`
price them into the card height; `gridView` computes the grid-wide maxima
once per frame (`m.gridBadgeLines`/`gridVersionLines`/`gridTechLines`) and
both the virtual-list height callback and `fitCards` consume them.
`textWidthAt`/`shapedGlyphsAt` generalize the text measure to the 11 px
pill font. Tests (internal/gui/pillwrap_test.go): greedy-wrap unit cases,
long-fork line estimates, a headless grid render asserting wrapped pill
row + grown uniform `cardH` + no clipping, and a detail-panel wrap render.
TDD: compile-red witnessed (undefined helpers/seam), then behavioral red
(pill row 19.5 px single-line, `cardH = 514, want > 514`), then green;
vet/gofmt clean. Development overlapped issue 014 (progressive scan
rendering) landing in the same tree; 014's committed regressions in the
GUI tests blocked this issue's green-gate commit and were fixed as issue
016. Commit hash pinned in a follow-up commit.
