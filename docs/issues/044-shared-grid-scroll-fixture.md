---
title: Shared 14-game grid fixture for the scroll/settle tests
description: Test-only refactor from the post-commit ponytail review — the 14-game steam fixture was duplicated verbatim between gridscroll_test.go and churn_probe_test.go; extracted as fourteenGameLibrary.
---

# 044 — Shared 14-game grid fixture for the scroll/settle tests

## What and why

Ponytail review of the issue-037/040 test diffs found the 14-game fake
steam library fixture duplicated verbatim in two test files. Extracted
as `fourteenGameLibrary` in gridscroll_test.go; both tests call it.

## Acceptance

- [x] `TestGridClickLastRowKeepsScroll` and `TestScrollChurnProbe` pass
      unchanged against the shared fixture.
- [x] Full `go test ./...` green (except the foreign session's
      hardware-gated `tmp/gpuprobe` probe, which cannot see /dev/dri
      from a sandboxed shell), `go vet`/`gofmt` clean.

## Outcome

Done. Net −10 lines, no behavior change. Commit: dcb6146.
