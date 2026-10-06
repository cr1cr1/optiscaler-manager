---
title: Tabbed GUI settings modal with a dedicated Optiscaler tab
description: The GUI settings modal gains a General/Optiscaler tab bar; all OptiScaler-specific settings (default version, distribution sources, bundle cache) move to the Optiscaler tab.
issue: 6
status: done
---

# 6 — Tabbed GUI settings modal with a dedicated Optiscaler tab

## What and why

The GUI settings modal had grown into one long scroll of unrelated
sections (library view, directories, launch template, umu-launcher,
OptiScaler version, fork sources, cache). Grouping by concern with a
tab bar lowers cognitive load and gives OptiScaler-specific settings a
single home. Scope: the GUI modal only; the TUI settings screen already
groups by focusable lists.

Design: a pill tab strip under the modal title (recessed `bgCard`
track, active tab raised in `bgRaised` with accent-tinted bold label —
the sidebar's active language), two tabs. **General**: Library (online
lookups, card size), Scan Directories, Launch Template, umu-launcher.
**Optiscaler**: Version (default version input), Sources (forks),
Cache (clear bundle cache). Apply/Close stay in the shared footer since
`applySettings` commits buffers from both tabs. Tabs are focusable and
answer Enter/Space; Left/Right on a focused tab switches tabs (the
global arrow handlers are muted while the modal is open). The modal
always opens on the General tab.

## Acceptance

- [x] Tab bar with General/Optiscaler; all OptiScaler-specific settings
  (default version, sources, clear cache) only on the Optiscaler tab.
- [x] Keyboard: trap auto-focuses the General tab on open, Tab/Enter
  and Left/Right switch tabs; full focus cycle verified per tab.
- [x] `go test ./...` green, `go vet`/`gofmt` clean.

## Outcome

TDD: `internal/gui/settings_tabs_test.go` written red first (default
tab, keyboard switch, arrow switch, General-tab cycle order), plus the
three existing focus-cycle tests updated to the new tab order
(`settings_test.go`) and the forks render test switched to the
Optiscaler tab (`forks_test.go`). Implementation: `settingsTab` state
on the model (reset in `openSettings`), `settingsTabBar` /
`settingsTabButton` / `settingsGeneralTab` / `settingsOptiscalerTab` in
`chrome.go`. Visual verification via rendered PNGs surfaced a
pre-existing clip (the 45-char DLSSNR slug + pattern + buttons overflowed
the modal); fork rows are now two-line (slug + actions, pattern below).
All green; `go vet` and `gofmt` clean. TUI untouched by design.
