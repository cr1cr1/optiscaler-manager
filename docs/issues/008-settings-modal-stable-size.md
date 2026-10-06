---
title: Settings modal keeps its height; empty inputs keep their hint on focus
description: Fix the settings modal resizing when switching tabs, and the add-fork glob input shrinking when focus hides its placeholder hint.
issue: 8
status: done
---

# 8 — Settings modal keeps its height; empty inputs keep their hint on focus

## What and why

Two layout-stability bugs in the GUI settings modal:

1. **Modal resized on tab switch.** The modal card is content-height
   driven; the General tab (417px of content) is taller than the
   Optiscaler tab (304px), so switching tabs visibly shrank the window.
   Fix: the tab-content wrapper carries `MinHeight(tallest seen this
   open)` — measured each frame from its rendered rect, reset on
   `openSettings` — so the modal holds one height across tabs while
   still growing if content does (adding fork rows, more scan dirs).
2. **Glob input shrank on focus.** `themedInputState` rendered the
   placeholder hint only when unfocused; focusing an empty field swapped
   the hint (~210px) for a bare caret, collapsing the box to its 180px
   minimum. Fix: an empty field renders the hint whether focused or not
   (the focus border is the affordance); the caret appears once text
   exists. This is the shared `themedInput`, so every settings/search
   field gains the stability.

## Acceptance

- [x] Content height identical across tabs
  (`TestGUISettingsModalKeepsHeightAcrossTabs`: red at 303.5 vs 417.3).
- [x] Input box width identical unfocused-hint vs focused-empty
  (`TestEditFieldWidthStableAcrossHintAndFocus`: red at 209.9 vs 180).
- [x] `go test ./...` green, `go vet`/`gofmt` clean.

## Outcome

Seams added first (`settingsContentRect`/`settingsContentMinH` on the
model, `boxRect` on `editState`) to witness both behavioral reds, then
the MinHeight max-tracking and the hint-stays-when-empty change. The
tab functions were flattened to render into the modal's measuring
wrapper (no nested gap containers). Visual verification via rendered
PNG: Optiscaler tab holds General's height with the footer at the same
position; the focused glob field keeps its hint and width. The
focused-empty caret is gone by design — the blue focus border signals
editability.
