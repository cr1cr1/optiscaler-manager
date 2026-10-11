---
title: Fossilize-replay compile backend for Warm
description: Validate fossilize_replay (ships with Steam) against the host NVIDIA/proprietary Vulkan driver and, if viable, add it as a second CompilerBackend behind the Warm button — deterministic headless compile, no game launch.
---

# 080 — Fossilize-replay compile backend for Warm

## What and why

Issue 078's spike proved `fossilize_replay` ships with Steam and runs,
but could not validate a replay on host hardware (sandbox: no
`/dev/dri`, no driver). Warm v1 is launch-and-record behind the
`CompilerBackend` seam (`WarmResult{BytesGrown, Elapsed, StopReason}`).
This issue validates the deterministic alternative: replaying recorded
Vulkan pipelines headless against the active driver (proprietary NVIDIA
615.71 here, not RADV — the win is unmeasured).

## Acceptance

- [ ] `fossilize_replay` replays a real game's recorded archives to
      completion on the host GPU (driver, device index, threads noted).
- [ ] Per-game archive availability mapped (steam fossilize layer
      output, `shadercache/<appid>` contents, which games have any).
- [ ] If viable: `FossilizeBackend` implementing `CompilerBackend`
      (same `WarmResult`, `StopReason: complete`), selected automatically
      when archives exist, launch-and-record otherwise. If not viable:
      close with the measured reason.
- [ ] `go test ./...`, `go vet`, `gofmt` green; docs updated.

## Outcome

Not started.
