---
title: TUI restore opens a backup-picker modal
description: Replace the staged restore cycle with a modal listing the game's DLSS backups; esc closes without action; dim the entry when there are no backups.
issue: 3
status: done
---

# 3 — TUI restore opens a backup-picker modal

## What and why

On the detail screen `p` currently stages a one-candidate restore pick that
`p` cycles through invisibly. A visible list is clearer: `p` opens a
bubbletea modal listing every DLSS backup (newest first), up/down moves the
selection, enter dispatches the highlighted snapshot (the session confirm
gate still guards the write), and esc closes without any action. Other keys
are swallowed while the modal is open. When the game has no backups, the
restore menu entry renders dimmed instead of the key silently doing
nothing.

## Acceptance

- [x] `p` on the detail screen opens the modal listing the game's backups.
- [x] j/k or up/down move the selection (no wrap); enter dispatches the
      highlighted snapshot through the session confirm gate; esc closes
      with no op raised.
- [x] Other keys do nothing while the modal is open.
- [x] With no backups the restore line renders dimmed and `p` is a no-op.
- [x] A row that is not DLSS-ready never opens the picker (the menu dim
      and the key share one gate).
- [x] An open picker re-syncs when a settled op event for the game
      arrives; a long backup list is windowed around the selection.
- [x] Full suite green; docs updated.

## Outcome

Done: `p` opens `restoreBox`, a centered bubbletea modal (same
`styleModal`/`lipgloss.Place` chrome as the confirm box, hand-rolled list —
`bubbles/list` is not vendored). The old staged restore cycle
(`stagedCycle.restore`, the `p`-advance branch, the staged line in the
detail view) is deleted; `stagedCycle` is version-only now. The backup list
is cached in `Model.backups` on detail entry and refreshed when a settled
op event for the detail game flows through `Update` — never per frame; the
refresh re-syncs an open picker, `restoreBox` windows the list to eight
rows around the selection, and `p` shares the menu's dim gate
(`canRestore`: DLSS-ready plus at least one cached backup).

Review fixes folded in: `ui.Session.opFailed` now tags `EvOpFailed` with
the game dir (matching `opRefused` and the launch emitter), which makes the
TUI's failure-event refresh leg live instead of dead. Red-proofs witnessed
by sabotage for the DLSS-ready gate, the settle re-sync, and the list
window; the selection-bounds, no-op-before-consent, and detail-entry-cache
pins are characterization tests. Committed with the docs as one change
(4d195cc).
