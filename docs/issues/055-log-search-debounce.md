---
title: Debounce log search typing by 300ms
description: The log panel's search highlight follows the field buffer only after 300ms of typing pause; Enter/lupe flushes immediately.
status: done
---

# 055 — debounce log search typing by 300ms

## What and why

Issue 054 made the live buffer the highlight query: every keystroke
recomputes match spans on every visible line. Typing should be debounced
— the highlight (the applied search query) updates only after a 300ms
typing pause.

## Design

- `m.logSearchBuf` stays the immediate field buffer; a new
  `m.logSearchQuery` is the *applied* query handed to `LogViewSearch`
  (highlight) and used by `logSearchNext`.
- Per frame, after the input renders: if the buffer changed since last
  frame, stamp `logSearchDirtyAt` and mark pending; while pending the
  pulse ticker (issue 040 mechanism) keeps frames coming.
- When pending and 300ms elapsed without further changes: promote
  buffer → query, clear pending.
- `logSearchNext` (Enter / lupe) flushes the debounce immediately, then
  jumps — an explicit search never waits.
- Empty buffer debounces the same way (highlight clears after 300ms).

## Acceptance

- [x] Typing (buffer change) does not change the applied query
  immediately; after a 300ms pause it does (`TestLogSearchDebounce300ms`).
- [x] Enter or the lupe button applies the current buffer without
  waiting (`TestLogSearchNextFlushesDebounce`).
- [x] `go test ./...` and `go vet ./...` pass (32/32 in an isolated
  worktree; the shared tree was mid-edit by the parallel session).

## Outcome

Done in commit a5fec9f. `logSearchQuery` is the applied highlight query handed to
`LogViewSearch`; `syncLogSearchQuery` (called per frame after the input
renders) re-arms a 300ms timer on every buffer change, keeps the pulse
ticker awake while armed, and promotes buffer → query after the pause.
`logSearchNext` flushes the debounce before jumping. Red witnessed as
build failures on the new seams, then green. Commit hash recorded in a
follow-up commit.
