---
title: Open-game-folder button in the detail panel
description: The GUI detail panel gains a button that opens the game's binary directory in the OS default file manager.
---

# 027 — Open-game-folder button in the detail panel

## What and why

The detail panel can edit a game's OptiScaler.ini but cannot take the
user to the game files themselves. A new **Open game folder** button
(SymFolder icon) opens the game's binary dir — the injection dir when
the row knows it (that is where the exe and the OptiScaler files live),
else the game root — in the platform's default file manager:
`xdg-open` on Linux, `open` (Finder) on darwin, `explorer` on windows.

The button is **not** install-gated (unlike OpenINI): the folder exists
regardless of OptiScaler state. The new `openFolder` launcher sits next
to `openExternal` in `internal/ui/browse.go` — deliberately separate:
`openExternal` is a terminal-EDITOR path for files, not a file manager.
The session seam (`Session.openFolder`) mirrors `openExternal` so tests
capture opens without spawning anything. Missing dirs, unknown games,
and launcher failures all surface as warn toasts.

## Acceptance

- [x] `Session.OpenGameFolder` opens the injection dir (game root
      fallback); missing dir / unknown game / opener error → warn toast,
      no spawn.
- [x] Detail panel renders the button for a clean (not installed) row —
      proven via the `openFolderRect` seam (mirrors `openINIRect`).
- [x] Docs updated: `docs/scope.md`, `docs/log.md`.

## Outcome

Done. TDD red witnessed as build failure on the new tests (undefined
`OpenGameFolder`/`openFolder`/`openFolderRect`), then green. Full
`go test ./...` exit 0 and `go vet`/`gofmt` clean on the committed tree
(the concurrent session's in-progress red-phase test files were held
aside for the verification run — they are not part of this commit).
Deferred: no TUI key binding (the request was a GUI button); no
click-through GUI test (no headless click harness for panel buttons —
same coverage level as OpenINI). Commit: see git log.
