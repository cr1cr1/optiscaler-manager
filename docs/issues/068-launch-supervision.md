---
title: Process supervision for launches (stop, exit reporting)
description: Routed launches are supervised — umu launches run under a per-game launch context (CancelOp kills the whole process group), owner-CLI launches are group-killable by pid; statuses report Running/exited/Stopped instead of a post-exit "Launch requested".
status: done
---

# 068 — process supervision for launches

## What and why

From the umu design notes: "Since umu blocks until exit, run it as a
supervised child. Track the process group. Kill on stop." Before this,
`Session.Launch` was bare `go doLaunch()` — no op slot, no stop, and
the blocking umu hook meant "Launch requested" appeared only after
the game exited.

## Acceptance

- [x] `umu.Launch` children get their own session; context cancel
  kills the whole process GROUP (umu-run → pressure-vessel → wine →
  game), `WaitDelay` bounds pipe waits. Proven against a real
  sleep-tree.
- [x] Session launch registry: `LaunchBusy`, double-launch guard,
  `CancelOp` falls through install op → launch slot → deps stopper.
  Status flow: Running → exited / Stopped / Failed (with log path).
- [x] `launchercli.Launch` returns the pid; the router registers it
  and returns a stopper (wired as `Deps.UmuStop`) that group-kills
  owner-CLI launches.
- [x] GUI: detail-panel Stop button while a launch runs. TUI: the
  existing `c` key stops launches (hint updated).
- [x] `go vet`, `go test ./...` green; darwin/windows cross-compile OK.

## Outcome

Limitations documented: wineserver stragglers inside the container may
escape the group kill (umu design note — `wineserver -k` is the manual
cleanup); owner-CLI double-launch is not guarded (the pid proves
spawn, not survival); a stopped mid-download umu first-run may leave a
partial runtime (umu re-resumes). Zombie-aware /proc checks in tests
(Released children are reaped on init's schedule). Commit:
41dacb7.
