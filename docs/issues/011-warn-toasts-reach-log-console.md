---
title: Warning toasts also reach the log console
description: Every user-facing warning/error (warn toasts, op failures, refusals) must also be written to the zerolog output — stderr for CLI/GUI, tui.log for the TUI — not only shown in the UI.
---

# 011 — Warning toasts also reach the log console

## What and why

User-facing warnings and errors surface as warn toasts
(`Session.toast(text, true)`), but `toast` never wrote to zerolog: only
call sites that remembered to pair a `log.Warn` left a trace, so failures
like "operation already in progress", the not-managed refusal, fork
validation errors, or `opFailed` itself were invisible in the log
console (stderr for CLI/GUI, `tui.log` for the TUI). Diagnosing a
user report from the log alone was guesswork.

The fix is structural, at the single funnel: `toast(text, warn)` with
`warn == true` now also emits `log.Warn()`. Every warning path —
`opFailed`, `opRefused`, `launchFailed`, settings/fork validation,
busy refusals, switch-chain aborts — is covered automatically, and
future call sites cannot forget. Call sites that already log a
structured `log.Warn().Err(err)` keep it: that entry carries the
machine detail (error, fields), the toast entry carries the exact text
the user saw; the two complement rather than duplicate. Info toasts
(success, "Cancelled") are not logged — the console stays signal-dense.

## Acceptance

- [x] A warn toast produces a matching `warn` entry in the zerolog
      output; an info toast produces none.
- [x] The `opFailed` path lands the underlying error text in the log.
- [x] `go test ./...`, `go vet`, `gofmt` clean; docs updated.

## Outcome

Done. `Session.toast` mirrors warn toasts to `log.Warn()` at the funnel
(three lines plus the rationale comment); structured `log.Warn().Err`
call sites unchanged (complementary detail); info toasts unlogged.
Tests capture the global logger into a buffer (sequential package, swap
restored on cleanup) and pin: warn toast → warn-level log entry, info
toast → none, `opFailed` → error text logged. TDD reds witnessed before
the fix; full suite, vet, gofmt green. Commit `bf7d2b9`.