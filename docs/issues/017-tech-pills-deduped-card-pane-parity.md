---
title: Tech badge pills deduplicated; same pills on card and detail pane
description: Cards showed tech badges that duplicate component pills (DLSS twice) and the detail pane showed none; both now render the same deduplicated techPills set.
---

# 017 — Tech badge pills deduplicated; same pills on card and detail pane

## What and why

User report after issue 015: "now there are more pills in the cards only,
some of them duplicated. The same pills must be in both card and details
pane." Once wrapping (015) made every pill row fully visible, two latent
problems surfaced:

1. **Duplicates on the card**: the tech badge row (`DLSS`, `FSR`, `XeSS`
   from game tech detection) duplicates the component pills
   (`DLSS 3.7.20`, `FSR 3.1` — same tech, with its version) whenever a
   game has installed components.
2. **Card/pane asymmetry**: the detail pane never rendered the tech badge
   row at all.

Both surfaces now render `techPills(e)`: the row's tech badges minus any
badge duplicated by a component pill (a component equals the badge label
or starts with it plus a space — so `DLSSG 2.1` does NOT drop the `DLSS`
badge). Badges with no matching component stay: detected tech with no
installed component is real information. The detail pane gained the tech
badge row (same `Wrap` layout), so card and pane always show the same
pill set. The card-height estimator (015) consumes the same filtered set
via `techBadgeWidths`.

Test note: the panel parity test renders at 900x800 (panel at minimum
width, taller viewport) because the virtualized panel culls rows below
the fold — at 1100x700 the wrapped version row pushes the tech row past
the viewport bottom and the rect seam reads zero.

## Acceptance

- [x] A tech badge whose tech already has a component pill is not
      rendered (card and pane); `DLSS`/`DLSSG` prefix boundaries respected.
- [x] The detail pane renders the same tech badge row as the card.
- [x] A fully-duplicated tech row renders nothing (no empty row).
- [x] Full `go test ./...` green; `go vet`/`gofmt` clean.

## Outcome

Done. `techPills` (internal/gui/widgets.go) filters the badges; gameCard
(internal/gui/grid.go) and detailPanel (internal/gui/view.go) both render
it (the pane's row is new); `techBadgeWidths` estimates from the filtered
set; `m.techPillRowRect` seam (reset per card, reset at panel start so a
missing panel row reads zero, never the card's stale capture). Tests in
internal/gui/techpills_test.go: dedupe unit cases incl. the DLSSG
boundary, card skip on full duplication, panel parity. Verified with a
before/after headless render (second duplicated pill row gone from the
card) and full suite (29 packages) exit 0. Commit `7f2c090`.
