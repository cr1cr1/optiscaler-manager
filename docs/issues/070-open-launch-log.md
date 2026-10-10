---
title: Open the per-launch log from the frontends
description: The newest per-launch log opens in the platform file handler (terminal editor on Linux) from the GUI detail panel ("Launch log" button) and the TUI detail screen (L); a game with no launches gets a toast.
status: done
---

# 070 — open the per-launch log from the frontends

## What and why

Issue 061 made launch diagnostics complete, but reaching them meant
finding the file by hand. With the log panel landed (issues 054/055),
the missing piece was the per-launch FILE affordance.

## Acceptance

- [x] `Session.OpenLaunchLog(gameDir)`: the newest
  `<slug>-<timestamp>.log` under `Deps.LogsDir` (production:
  `<data root>/logs`) opens via the `openExternal` seam (terminal
  editor on Linux); missing logs and unknown games toast.
- [x] GUI: "Launch log" button in the detail panel's action row.
- [x] TUI: `L` on the detail screen (hint updated).
- [x] `go vet`, `go test ./...` green; darwin/windows cross-compile OK.

## Outcome

Small by design: the affordance reuses the browse seams; the launch
log itself (header + full output) stays the source of truth. Commit:
dee8e88.
