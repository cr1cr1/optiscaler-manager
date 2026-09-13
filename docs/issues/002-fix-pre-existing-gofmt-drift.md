---
title: Fix pre-existing gofmt drift in untouched files
description: Eight files fail gofmt -l; all predate the v0.16 work.
issue: 2
status: open
---

# 2 — Fix pre-existing gofmt drift in untouched files

## What and why

`gofmt -l internal/` flags internal/gui/cursor_test.go,
internal/gui/model.go, internal/pickdir/pickdir_windows.go,
internal/tui/model_test.go, internal/ui/coverfallback_test.go,
internal/ui/disable_test.go, internal/ui/rows.go, and
internal/umu/runners_linux_test.go. The drift predates the v0.16 work
and was left untouched there to keep those diffs minimal. Pure
formatting, no behavior change.

## Acceptance

- [ ] `gofmt -l internal/` prints nothing.

## Outcome
