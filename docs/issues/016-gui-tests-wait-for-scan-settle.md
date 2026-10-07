---
title: GUI tests wait for scan settle, not row count (issue 014 follow-up)
description: Scan streaming (014) made rows visible long before the scan settle; GUI tests that waited for a row count raced the settle and failed. They now wait for the terminal event plus a quiet period.
---

# 016 — GUI tests wait for scan settle, not row count (issue 014 follow-up)

## What and why

Issue 014 (progressive scan rendering) streams rows into state as they are
discovered — cover-less, in discovery order, mixed with `AddDirectory`
placeholder rows. GUI tests written against commit-at-settle timing waited
for a row COUNT (`len(VisibleRows()) >= n`), which now fires mid-scan:
`TestGUIArrowKeyNav` snapshotted the row order before the settle's sort,
the four `TestPanelTab_*` tests read a placeholder row with empty status
instead of the settled `external`, and dropdown/DLSS tests flickered
whenever trailing events landed mid-assertion. The failures were proven
pre-existing on the issue-014 commit (`6e9920f`) with all issue-015 work
stashed; 014's own verification could not see them because 015's
compile-red test file blocked the `internal/gui` build at the time.

All row-count waits now go through `waitScanSettled`: it drains events
until `EvScanDone` (a dropped terminal event is covered by a
scan-started-then-quiet poll of `Busy`/`Progress`) plus a trailing 300ms of
event silence that absorbs the async `AddDirectory` enrichment goroutines,
then asserts the settled row count. Two `Select`-wait loops in
dropdown_test.go got a second, latent fix: `Session.Select` emits no
event, so their "drain only on events" loops only worked while leftover
scan events remained in the buffer — they now drain every tick.

## Acceptance

- [x] `TestGUIArrowKeyNav`, `TestPanelTab_*` (4),
      `TestVersionDropdown_*`, `TestDLSSControl_*` and the grid/list focus
      tests pass deterministically (three consecutive full-package runs).
- [x] No production code changed — the streamed-rows design of 014 is
      unchanged; only test synchronization.
- [x] Full `go test ./...` green; `go vet`/`gofmt` clean.

## Outcome

Done. `waitScanSettled`/`settledRows` (internal/gui/gui_test.go) replace
seven naive wait loops (scanOneRow, seedNavSession, TestGUIArrowKeyNav,
seedExternalPanelSession, TestGrid_TabOrderCardThenInnerItems,
TestListRows_DoNotOverlap, the sort test); the two `Select`-wait loops in
dropdown_test.go drain every tick. Verified: three consecutive
`internal/gui` runs green, then full `go test ./...` (29 packages) exit 0;
vet/gofmt clean. Commit `6233e4f`.
