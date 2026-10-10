---
title: umu route never runs against a launcher-owned prefix
description: A forced "Launch via: umu" with a Bottles/Lutris/Heroic/Steam/PoL/CrossOver prefix is now a launch error naming the owner and remedy — umu mutates any Proton prefix it launches, so foreign-prefix corruption is blocked structurally, not just by auto-routing.
status: done
---

# 062 — umu-route prefix guard

## What and why

Issue 060 made *auto* route owned prefixes to their owner CLI, but a
forced `umu` route (e.g. the global default) still ran umu-run against
whatever was pinned — including Steam `compatdata` or a Bottles bottle.
umu mutates any Proton prefix it launches (DLL symlinks, `pfx/`
layout); the reporter's Bottles prefixes already needed repair once.
The guard makes foreign-prefix corruption structurally impossible.

## Acceptance

- [x] Forced umu route + launcher-owned prefix (scan source bottles /
  lutris / heroic / steam / playonlinux / crossover) → launch error
  naming the prefix, the owner, and the remedy (pin an app-managed/umu
  prefix or set Launch via: auto/owner). umu-run is never invoked.
- [x] `~/.wine` and unrecognized dirs (no launcher owner) stay
  umu-eligible.
- [x] `go vet`, `go test ./...` green.

## Outcome

The guard lives in the router's umu branch (cmd/umu_linux.go), keyed
on `LookupPrefix`'s ownership scan. The issue-059 router-level test
(forced umu + Bottles prefix picks the Bottles runner) was
premise-invalid and deleted; runner-follows-prefix-source stays
covered at the `ResolveProtonPath` level. Commit: 978660c.
