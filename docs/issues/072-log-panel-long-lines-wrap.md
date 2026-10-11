---
title: Log panel wraps long lines; long status text can no longer stretch the layout
description: A long single-line status inflated the main column's intrinsic width past the window; shirei's grow-only distribution fed the poisoned width back into the log view's wrap budget, so log lines rendered unwrapped and clipped at the window edge, and extreme lines made the whole main view horizontally scrollable.
---

# 072 — Log panel wraps long lines; layout immune to long text

## What and why

User report: "log panel expands main view when very long lines; must not
happen, also long lines must be wrapped."

Root cause (reproduced headless with real font metrics): the status bar's
status label (`muted(oneLineStatus(...))`) is an unbounded text leaf — no
MaxSize, no wrap budget, no clip. A long single-line status (e.g. a 205-char
failure message ≈ 1205 px) makes the status row's intrinsic width exceed the
window. shirei's top-down distribution only *grows* children
(`roomForGrowth > 0` gate in `resolveSizesFromOutside`), never shrinks them,
so the inflated intrinsic propagates to the main column and every `Expand`
child. Screen rects are clamped to the window afterwards, but the poisoned
`resolvedSize` is what `GetResolvedSize()` reports — and the LogView's
virtual list derives its item/wrap width from exactly that (1613 px instead
of 1012 px). Log lines therefore shape with a budget far wider than the
visible panel: no wrapping, text clipped at the window edge. With even
longer lines the root Row's content overflows and the whole main view
becomes horizontally scrollable — the reported "expands main view".

Second wrap gap: `lineBreakShapedSegments` breaks only *between* segments
(words/spaces), so a single unbreakable token longer than the budget (URLs,
hashes, spaceless paths) never wraps at any width.

Fix, three layers:

1. Status bar: ellipsize the one-line status to the measured available
   width (bar rect from the previous frame minus the measured width of the
   Log button, hints, and game count), and clip the bar's row. The full
   text remains in the log panel.
2. Vendor v0.24 (`scroll.go`): a virtual list's item width is clamped to
   the container's on-screen width when that is known — poisoned
   `resolvedSize` feedback can never mis-shape list items again (any list,
   not just the log).
3. Vendor v0.24 (`text.go`): segments that alone exceed the wrap budget are
   split at glyph-cluster boundaries, so unbreakable tokens wrap too.
   Selection/search span math is unaffected (glyph clusters and rune
   offsets are preserved).

## Acceptance

- [x] A game library with a very long status line keeps the log panel's
      wrap budget equal to its visible width (log lines wrap; the hints and
      game count stay on screen; no horizontal overflow of the main view).
- [x] A log line consisting of one long unbreakable token wraps in the
      panel instead of clipping at the window edge.
- [x] Word-wrapped log lines, search highlight, selection, pin-to-bottom,
      and the grid's virtual list behave as before (full suite green).
- [x] `go test ./...`, `go vet`, `gofmt` green; docs updated.

## Outcome

Implemented as designed; root cause confirmed by instrumented headless
reproduction before any fix (probes deleted after use).

- Diagnosis trail: with real font metrics (`WaitForSystemFontScan` unlocks
  them in tests), a 205-char status line resolves the log panel's width
  feedback to 1613 px while its on-screen rect stays 1028 px; log items
  shape at the poisoned width, so nothing wraps. Screen rects stay correct,
  which is why the layout looked bounded while the text clipped.
- `internal/gui/chrome.go`: `statusBar` ellipsizes the one-line status via
  the new `fitStatusLine` (binary-searched prefix + "…" against the
  measured free width: bar rect minus the measured Log button, hints, and
  game-count widths from the previous frame) and clips the bar row.
- Vendor v0.24: `widgets/scroll.go` clamps every virtual list's item width
  to the container's on-screen box (defense in depth — any future wide
  label can no longer mis-shape lists); `text.go`
  `splitOverlongSegments` glyph-breaks tokens that alone exceed the wrap
  budget, so unbreakable strings wrap too.
- Red-first tests: `TestLongStatusLineKeepsLogWrapBudget` (resolved width
  parity, new `logPanelResolvedW` seam), `TestFitStatusLineEllipsizes`,
  `TestShapeTextMaxBreaksOverlongToken` (both font-guarded),
  `TestVirtualListWidthClampedToVisibleBox` (font-free synthetic poison).
  Vendor markers guarded in `TestVendorCSDPatchPresent`.
- Post-fix render verified visually (PNG): wrapped log lines, char-wrapped
  600-char token, ellipsized status with hints visible, centered empty
  state (previously pushed off-screen by the poisoned layout).
- Docs: vendor-patches.md (v0.24 entry), scope.md (Chrome bullet).
- `go test ./...` 35/35 ok, vet + gofmt clean.

Commit: a763e8b.
