---
title: Preflight checks for umu launches
description: umu launches fail fast with errors naming the broken piece — invalid runner (no executable proton script), unwritable prefix, stale exe, or first-run runtime download with no network — instead of dying inside umu-run's Python or hanging.
status: done
---

# 065 — preflight checks for umu launches

## What and why

From the umu design notes: "Preflight checks: umu-run exists, Proton
path valid, prefix writable, network or cached runtime available."
Only the umu-run check existed. The debugging loop showed why the
rest matter: a read-only prefix surfaced as an `OSError` traceback
deep inside umu's Python, and a first-run runtime download with no
network hangs for minutes with no output.

## Acceptance

- [x] `umu.CheckProton`: pinned/auto runner must be a dir with an
  executable `proton` script; empty stays valid (umu's own default).
- [x] `umu.CheckPrefixWritable`: the prefix or its deepest existing
  ancestor must accept a temp file (umu writes `pfx.lock` at the
  root).
- [x] `umu.CheckExe`: the game exe must exist (stale ExePath →
  "rescan the game").
- [x] Runtime-or-network: with `steamrt3` not yet downloaded, a 2s
  TCP probe (`umuNetProbe`, stubbed in tests) decides — unreachable
  is an error naming both; reachable logs "first launch fetches it,
  expect a long run".
- [x] All run BEFORE umu-run is spawned; errors name the broken piece
  and the remedy.
- [x] `go vet`, `go test ./...` green; darwin/windows cross-compile OK.

## Outcome

Test fixtures had to grow up: launch tests now create real exe files
(`touchExe`), valid pinned runners, and writable prefixes; the probe
is stubbed in `fakeUmuRun` so no test touches the network. Commit:
THIS_COMMIT.
