---
title: Fix pre-existing gofmt drift in untouched files
description: Eight files fail gofmt -l; all predate the v0.16 work.
issue: 2
status: done
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

- [x] `gofmt -l internal/` prints nothing.

## Outcome

Closed in the same session. `gofmt -w internal/` fixed all eight files;
the change is whitespace only (+12/−16 lines): removed trailing blank
lines, re-indented a misaligned comment inside a composite literal,
re-aligned two struct field blocks and one const block, and dropped
double blank lines. Gates after the fix: `gofmt -l` clean on `internal/`,
`cmd/`, and root, `go vet ./...` clean, full uncached `go test ./...`
(29 packages) ok, golangci-lint 0 issues. Committed as efbf4d0.
