---
title: Pill hover tooltips spell out label abbreviations (500ms debounce)
description: Hovering a pill whose label leads with a known abbreviation (DLSS, DLSS-FG, FSR, XeSS, EAC) floats its full name next to the pill after a 500ms debounce; works on cards and in the detail pane.
---

# 020 — Pill hover tooltips spell out label abbreviations (500ms debounce)

## What and why

User request: "Add tooltip when hovering on pill, with the full name of the
pill label abbreviations (DLSS, DLSS-FG, FSR... etc)", then "debounce
tooltip show to 500ms". shirei has no tooltip primitive, so the pill
registers the request during the frame and `rootView` renders the overlay
last — outside every `Clip` and virtualized viewport a pill may live in
(cards, the scrolling detail pane).

- `pillTipText(label)` resolves a pill label to a full name by its leading
  token (version suffix stripped): DLSS → NVIDIA Deep Learning Super
  Sampling, DLSS-FG → NVIDIA DLSS Frame Generation, FSR → AMD FidelityFX
  Super Resolution, XeSS → Intel Xe Super Sampling, EAC → Easy Anti-Cheat.
  Keys match the classified upscaler kinds (`internal/domain/release.go`)
  plus EAC. Plain names (Proton, Steam, fork pills) and unknown tokens
  (`DLSSG`) get no tooltip.
- `pillHoverTip(label)` is called inside the pill's container closure —
  wired into `badgePill` (covers tech badges, component pills, status/EAC
  pills on cards AND the pane) and into the interactive DLSS control's
  outer container (its label still leads with `DLSS`).
- The package-level `pillTip` state is re-armed per frame by `rootView`
  (`seen`/`shown` cleared) while `text`/`rect`/`since` persist, so the
  500ms debounce clock survives across frames: hovering the SAME pill
  (same text + rect) keeps the clock; a different pill or leaving all
  pills restarts it. Package-level because `badgePill` has no model
  handle; the GUI is a single instance rendering single-threaded.
- The overlay floats below the anchor pill (flips above near the window's
  bottom edge), horizontally clamped into the window, at Z(20) — over
  cards, panel, and toasts.

## Acceptance

- [x] Hovering an abbreviation pill on a card registers the tooltip; it
      renders only after the 500ms debounce elapses.
- [x] Same behavior in the detail pane (shared `badgePill`).
- [x] The interactive DLSS update/restore control tooltips too.
- [x] Mouse leaving every pill clears the state; re-hover debounces from
      zero again.
- [x] Full `go test ./...` green; `go vet`/`gofmt` clean.

## Outcome

Done. New `internal/gui/tooltip.go` (`pillTipText`, `pillHoverTip`,
`pillTipOverlay`, `pillTip` state); one-line hooks in `badgePill`
(internal/gui/theme.go), `dlssControl` (internal/gui/dlss.go), and
`rootView` (internal/gui/view.go). Tests in internal/gui/tooltip_test.go:
label→name mapping (incl. the DLSSG prefix boundary), card hover with
debounce gating + clear + re-hover re-debounce, DLSS control hover, pane
hover. TDD red witnessed (undefined `pillTip`/`pillTipState`), then green.

Test note: the debounce identity check compares the pill's rect, and the
first hover frames of an isolated test run can still reshape text (font
init), shifting the rect and legitimately restarting the clock — the hover
test renders three warm-up frames before backdating `pillTip.since`.

Deferred (ponytail): no keyboard-focus tooltip (hover-only, as requested);
headless `RenderToPNG` evidence shot skipped — its 2x physical-pixel
coordinate space mismatches logical-rect pre-seeding, and the behavior is
covered by the state assertions. Full suite (29 packages) exit 0.
Commit TBD.
