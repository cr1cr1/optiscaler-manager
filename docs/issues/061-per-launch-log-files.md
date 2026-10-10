---
title: Per-launch log files for umu and owner-CLI launches
description: Every routed launch writes a per-launch log under <data root>/logs (exact argv, env delta, full stdout/stderr; UMU_LOG=1 for umu); failures name the file. Groundwork for the launch debugging loop.
status: done
---

# 061 — per-launch log files

## What and why

Launch failures used to surface only a bounded stderr tail in a toast;
stdout was discarded and umu ran without `UMU_LOG`. Diagnosing "the
game does not start" was guesswork. Now every routed launch (umu-run
and owner CLIs alike) writes a per-launch log file, and the failure
error names it.

## Acceptance

- [x] `internal/launchlog`: slugged timestamped file creation, header
  writer (time, exact argv, env delta only — never the full inherited
  environment).
- [x] umu launches (`LaunchOpts.LogDir`/`LogName`): full stdout+stderr
  captured to the file, `UMU_LOG=1` set (ExtraEnv can override), errors
  carry `full log: <path>`.
- [x] Owner-CLI launches (`launchercli.Opts.LogDir`/`LogName`): the
  detached child's stdout/stderr captured the same way.
- [x] Production wires `<data root>/logs`; a file that cannot be
  created degrades to a warning, never a failed launch.
- [x] `go vet`, `go test ./...` green; darwin/windows cross-compile OK.

## Outcome

`launchercli.Launch` gained an `Opts` struct (was five positional
params). `newUmuLauncher` takes the log dir (cmd/session.go wires
`<DataRoot>/logs`). The stale "detaches in spawn_linux.go's
platformRunner" comment on the umu runner was corrected — the umu path
blocks on `cmd.Run()`, which is what makes full stderr capture there
possible. GUI log-panel integration deliberately untouched (owned by a
parallel effort); the log directory is the stable contract. Commit:
6000655.
