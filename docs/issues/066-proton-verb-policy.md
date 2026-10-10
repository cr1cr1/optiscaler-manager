---
title: PROTON_VERB policy for concurrent same-prefix launches
description: The first launch into a prefix uses waitforexitandrun; while a game still runs there, further launches use runinprefix — a second process no longer waits (looks hung) for the first game's wineserver.
status: done
---

# 066 — PROTON_VERB policy

## What and why

From the umu design notes: "Running a second process in the same
prefix (a launcher plus a game, or a store client plus a game) needs
PROTON_VERB=runinprefix on subsequent launches." We hardcoded
`waitforexitandrun` — a second launch into a busy prefix would wait
for the first game's wineserver and look hung.

## Acceptance

- [x] `LaunchOpts.Verb` (empty = `waitforexitandrun`) becomes
  PROTON_VERB.
- [x] The router tracks busy prefixes (registry lifetime = the
  blocking `umu.Launch` call, so it matches the game's): concurrent
  launch → `runinprefix`; after exit the next launch waits again.
- [x] Deterministic cmd test: gated fake umu-run overlaps two launches,
  asserts verb sequence wait → runinprefix → wait.
- [x] `go vet`, `go test ./...` green; darwin/windows cross-compile OK.

## Outcome

The registry lives in the hook closure (one per session); owner-CLI
launches are unaffected (the launcher's own verb handling applies).
Games launched outside the app are not tracked — documented
limitation. Commit: 3d98141.
