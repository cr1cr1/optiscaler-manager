---
title: GPU rendering default-on audit; correct stale darwin-blocker docs
description: Audit confirms vendored shirei v0.8.0 already composites on the GPU by default (Wayland/Windows/macOS); docs-only scope records the audit and fixes the obsolete "Apple SDK required" rationale.
---

# 041 — GPU rendering default-on audit; correct stale darwin-blocker docs

## What and why

User request: shirei v0.8 can do GPU rendering — enable it by default, for
all platforms (<https://judi.systems/shirei/blog/release-v0.8-2026-09-24/>).

Audit findings (vendored shirei v0.8.0, migrated in a60ea76):

- The GPU compositor is **already the default** on every backend that has a
  GPU path: Wayland (GLES/dmabuf), Windows (D3D11), macOS (Metal via purego,
  no cgo), Android, iOS, Web (WebGL2). The software renderer is only a
  fallback on init failure or via `SHIREI_GPU=0` — which this app never sets
  (no occurrence outside vendor). Nothing to enable app-side.
- Prior session telemetry (log.md, 2026-09-11) already proved the GPU path
  active on the user's Wayland session.
- **X11 stays on the software renderer by upstream design** (no X11 GPU
  backend exists); Wayland is the GPU path on Linux. Not addressable
  app-side.
- The v0.6.x-era "darwin blocked" rationale is **obsolete**: shirei v0.8.0's
  cocoa backend is purego (objc_msgSend via `ebitengine/purego` + assembly),
  Metal included, so `CGO_ENABLED=0 GOOS=darwin` builds (amd64 + arm64) and
  whole-tree darwin vet now pass (verified locally 2026-10-08). macOS
  artifacts could ship from a Linux runner today.

User decision: **docs-only scope** — record the audit and correct the stale
rationale; do NOT change release or CI config. macOS release artifacts
therefore remain disabled, but by decision, not by a technical blocker.

Changes: scope.md and architecture.md stop claiming macOS needs an Apple
SDK; architecture.md records the GPU-renderer reality (default-on per
backend, software fallback, `SHIREI_GPU=0` opt-out, X11 software-only);
log.md records the audit. README.md needs no change ("available for Linux
and Windows" remains factually true). No code or vendor change, so no new
behavior test.

## Acceptance

- [ ] scope.md, architecture.md no longer claim macOS is technically
      blocked; the docs record that darwin artifacts are disabled by
      decision while the CGO-free build path is verified working.
- [ ] architecture.md documents the GPU-compositor defaults and the X11
      software-renderer reality.
- [ ] log.md records the audit and the docs-only decision.
- [ ] `go test ./...`, `go vet ./...`, gofmt clean (no code change expected).

## Outcome

Filled in when the issue closes: what actually happened, the commit
hash, anything left open.
