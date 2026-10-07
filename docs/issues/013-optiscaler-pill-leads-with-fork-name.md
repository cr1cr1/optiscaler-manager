---
title: OptiScaler pill leads with the fork name
description: The version pill showed "OptiScaler vx.y.z · <ForkName>"; it now shows "<ForkName> <version>" directly, with upstream keeping "OptiScaler <version>".
---

# 013 — OptiScaler pill leads with the fork name

## What and why

With a fork selected, the OptiScaler pill read
`✦ OptiScaler 0.8.92 · OptiScaler-DLSSNR-PreSR-Multipass` — the actual
installed distribution was demoted to a suffix behind the generic
"OptiScaler" name. The pill must name the distribution directly:
`✦ OptiScaler-DLSSNR-PreSR-Multipass 0.8.92`.

`optiBadge` now derives the pill name from `GameRow.ForkLabel()` (the
fork's repo segment), falling back to "OptiScaler" for upstream, legacy,
and external rows — so upstream pills are unchanged
(`✦ OptiScaler 0.9.4`), external pills keep their `· external` marker,
and an unversioned fork row names the fork (`✦ OptiScaler-fork`).

The TUI detail panel's `Version: <version> · <fork>` metadata line is a
field, not the pill, and keeps its format.

## Acceptance

- [x] A fork install's pill is `✦ <ForkName> <version>` (purple,
      committed); upstream stays `✦ OptiScaler <version>`.
- [x] An unversioned committed fork row shows `✦ <ForkName>`.
- [x] External rows keep `✦ OptiScaler <version> · external`.
- [x] `go test ./...`, `go vet`, `gofmt` clean; docs updated.

## Outcome

Done. `optiBadge` (internal/gui/widgets.go) builds the label as
`"✦ " + name + " " + version` with `name = ForkLabel()` or the
"OptiScaler" fallback; the `forkSuffix` construction is gone. Test
`TestOptiBadgeForkNamedPill` pins fork, upstream, and unversioned-fork
pills; existing external-pill tests pass unchanged. TDD red witnessed
(`✦ OptiScaler 0.8.92 · OptiScaler-DLSSNR-PreSR-Multipass` before the
fix); full suite (29 packages), vet, gofmt green. Commit `15341aa`.
