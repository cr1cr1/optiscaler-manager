---
title: OptiScaler action labels, wrapping detail action row, settings Apply/Close row
description: Install/Uninstall buttons name OptiScaler everywhere, Launch becomes Launch Game, the detail pane's action buttons flow horizontally with wrap, and the settings modal's Apply/Close share a row.
---

# 033 — OptiScaler action labels, wrapping detail action row, settings Apply/Close row

## What and why

Button-label and layout pass across both frontends:

1. **Labels name the payload**: `quickLabel` now returns "Install
   OptiScaler" / "Uninstall OptiScaler" / "Adopt OptiScaler" (cards and
   detail pane share it), and Launch is "Launch Game" (card + detail).
   TUI detail actions match: "i install/uninstall OptiScaler", "i adopt
   OptiScaler (install over external)", "l launch game". The TUI footer
   key legend keeps its short forms — it is a shortcut hint, not a
   button, and lengthening it would overflow the footer.
2. **Detail pane action row**: all action buttons (install, launch,
   rollback, disable/enable, open folder, reset poster, open INI) live
   in one `Row + Wrap` container — one horizontal line on wide panes,
   reflowed lines on narrow ones, never one-per-line, never overflowing.
3. **Settings modal**: Apply and Close share a row instead of stacking.

## Acceptance

- [x] `quickLabel` captions (updated guards: TestQuickLabelExternal,
      TestQuickInstallButtonLabelByStatus).
- [x] Wide pane: Install/Launch/Open-folder on one line; narrow pane:
      wraps, nothing crosses the panel's right edge (seams:
      `quickBtnRect`, `launchBtnRect`, existing `openFolderRect`).
- [x] Settings Apply/Close same Y (seams: `settingsApplyRect`,
      `settingsCloseRect`).
- [x] TUI detail action labels (new TestDetailActionLabelsNameOptiScaler;
      TestDetailViewAdoptHintForExternal updated to the new adopt text).
- [x] Docs updated: `docs/scope.md`, `docs/log.md`.

## Outcome

Done. TDD red witnessed (undefined seams; old-label assertions failing).
One test first failed "install button not rendered" at 1600×1000 — the
2:3 cover pushes the action row past the fold and shirei culls clipped
Viewport children; tall windows (1400) as usual. Full `go test ./...`
exit 0 (31 packages), `go vet`/`gofmt` clean. Commit: f58fb3a.
