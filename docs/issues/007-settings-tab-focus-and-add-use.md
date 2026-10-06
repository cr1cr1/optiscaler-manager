---
title: Settings modal tab focus follows click; add-fork becomes add & use
description: Fix the settings tab buttons leaving the keyboard focus ring on General after a mouse click; rework the add-fork form into one row whose successful add also activates the new source.
issue: 7
status: done
---

# 7 — Settings modal tab focus follows click; add-fork becomes add & use

## What and why

Two GUI settings-modal problems reported after issue 6:

1. **Bug**: clicking the Optiscaler tab switched the content but left the
   keyboard focus ring on the General tab — the ring is the app's
   selection read-out, so the modal looked like "General stays
   selected". Root cause: `settingsTabButton` never called
   `FocusOnClick`, so pointer presses switched `m.settingsTab`
   (`PressAction` is focus-independent) without moving focus. Diagnosis
   proven by disabling `FocusOnClick` under
   `TestGUISettingsTabSwitchViaMouse`: the tab-state assertion passed,
   the focus assertions failed.
2. **Layout**: the add-fork form stacked slug input, glob input, and an
   "Add fork" button vertically, and activating the new source needed a
   separate "Use" click on its row.

Fixes: tabs call `FocusOnClick` (click moves focus, the ring follows
selection), and the active tab label switched from `accentHov` (HSL
lightness 34, too dim against `bgRaised` 25) to bright bold `txtMain`.
The add form is one row — slug input, glob input, "Add & use" — and a
successful add also activates the fork (`SetActiveFork` after
`AddFork`), so adding and applying a source is one action; the per-row
"Use" button remains for switching back.

## Acceptance

- [x] Clicking a tab switches it AND moves the focus ring to it
  (`TestGUISettingsTabSwitchViaMouse`).
- [x] Adding a fork activates it immediately, persisted
  (`TestGUISettingsAddForkActivates`).
- [x] Add form renders as a single row; full `go test ./...` green.

## Outcome

TDD: mouse-click tab test (new rect/id seams `settingsTabRects` /
`settingsTabIDs` on the model) and add-activates test red first.
`FocusOnClick` added to the tab buttons; active label contrast raised;
`settingsForksSection` add form re-laid as one row with an "Add & use"
button; `addForkFromBuffers` activates on success. README's GUI
settings paragraph rewritten for the two tabs. `go test ./...` green,
`go vet`/`gofmt` clean. TUI add flow unchanged (its two-input chain
already ends in an explicit activation step by list navigation).
