---
title: Grouped detail action rows, folder renamed to directory
description: The detail pane's action buttons form two rows — game actions (Launch Game, Game directory) above the OptiScaler actions — and every user-facing "folder" becomes "directory".
---

# 036 — Grouped detail action rows, folder renamed to directory

## What and why

1. **Two action rows in the detail pane**: row 1 holds the game actions —
   "Launch Game", then "Game directory" (plus the conditional
   "Reset poster", which is a poster/game action, not an OptiScaler
   one) — and row 2 holds every OptiScaler-related button:
   Install/Uninstall/Adopt OptiScaler, Rollback, Disable/Enable, and
   Open OptiScaler.ini in editor. Each row keeps its own `Wrap`, so a
   narrow pane reflows inside a row but the two groups never interleave
   (issue 033's single wrap row did interleave: on narrow panes Open
   folder wrapped *below* Install).
2. **"folder" → "directory" in all user-facing copy**: the Open-game
   button, both empty-state guidance lines (GUI and TUI), the
   missing-dir and scan-root toasts, and the settings directories
   placeholder. Internal identifiers (`openFolder`, `OpenGameFolder`,
   `openFolderRect`, `SourceFolder`, Steam's `libraryfolders.vdf`) keep
   their names — they are code, not copy.

## Acceptance

- [x] Wide pane: Launch Game and Game directory share row 1 (same
      Y, launch left of open-dir); Install sits strictly below (seams:
      `launchBtnRect`, `openFolderRect`, `quickBtnRect`).
- [x] Narrow pane: rows wrap but never interleave — Install stays below
      the game row and nothing crosses the panel's right edge.
- [x] Copy pins: `emptyStateCopy` says directory (GUI), the TUI empty
      frame says "add a directory", the missing-dir toast says "game
      directory not on disk", the container-add toast says "as a scan
      directory".
- [x] Docs updated: `docs/scope.md`, `docs/log.md`.

## Outcome

Done. TDD red witnessed (`tmp/issue36-red.txt`): install shared the
game row's Y in the wide pane and wrapped *above* Open folder in the
narrow pane; all four copy pins failed on "folder". One green-run fix:
at 900×800 the second action row falls below the Viewport fold and
shirei culls it (zero rect) — the narrow subtest now uses a tall
900×1400 window, same as the other action-row tests. Full
`go test ./...` exit 0 (32 packages), `go vet`/`gofmt` clean. Commit:
964f40d.
