---
title: Log lines prefixed with the game title in a distinct color
description: Game-scoped status lines in the log panel carry the game title as a colored prefix, so the history is scannable per game.
status: done
---

# 057 — game-title prefixes in the log panel

## What and why

The log panel (052/054/055) shows bare status lines; with several games
installed/launched in a session it is impossible to tell which game a
line belongs to. Game-scoped status lines get the game title as a
prefix, painted in a distinct text color.

## Design

- **ui**: the status ring becomes `[]LogEntry{Line, Game}` —
  `setStatus` stays global (Game ""), new `setStatusGame(gameDir, line)`
  resolves the row title. Converted call sites: `opDone`, `opFailed`,
  `opRefused`, `opCancelled`, `launchFailed`, `doLaunch` (both success
  texts). `LogLines()` is kept (derived) for compatibility; new
  `LogEntries()` exposes the pairs. Cap/dedupe rules unchanged.
- **gui**: the mirror syncs entries; the ring line text becomes
  `"Title › line"` (plain `line` when no game), and the prefix rune
  range is recorded per line for coloring.
- **Color**: vendor patch **v0.23** adds
  `widgets.LogViewSearchExt(..., extraSpans func(idx int) []StyleSpan)`;
  the app supplies a `TextColor: logGamePrefix` span over the prefix.
  Search highlighting (v0.22) composes via the same spans slice.
- Status-bar text is unchanged (single line, no prefix).

## Acceptance

- [x] A failed launch produces a log entry whose Game is the row title;
  global statuses (scan, cache) keep an empty Game
  (`TestLogEntriesCarryGameTitle`, `TestLogEntryCapKeepsTitlesAligned`).
- [x] The log panel renders `Title › line` with the prefix in
  `logGamePrefix` and the line in the normal text color
  (`TestLogEntriesRenderTitlePrefix`).
- [x] Search highlight still works on prefixed lines (spans compose;
  `TestLogSearchScrollsMatchIntoView` et al. green).
- [x] Vendor patch v0.23 marker guarded; `docs/vendor-patches.md`
  updated.
- [x] `go test ./...` and `go vet ./...` pass (32/32, main tree).

## Outcome

Done. Ring is `[]LogEntry{Line, Game}`; `setStatusGame` attaches the row
title at opDone/opFailed/opRefused/opCancelled/launchFailed/doLaunch;
the GUI renders `Title › line` with the prefix in `logGamePrefix` via
vendor patch v0.23 (`LogViewSearchExt` per-line extra spans).
Red witnessed as build failures on the new seams, then 32/32 green.
Commit hash recorded in a follow-up commit.
