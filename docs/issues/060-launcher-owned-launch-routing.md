---
title: Launcher-owned launch routing (umu vs owner CLIs)
description: Manual Windows games launch through the launcher that owns their prefix (bottles-cli, lutris, heroic); umu-run only for unowned prefixes; raw PE exec removed. Spec: docs/superpowers/specs/2026-10-10-launcher-owned-launch-routing-design.md
status: done
---

# 060 — launcher-owned launch routing

## What and why

Implements the spec at
`docs/superpowers/specs/2026-10-10-launcher-owned-launch-routing-design.md`:
one prefix = one owner, and the owner launches it. umu-run no longer
touches Bottles/Lutris/Heroic-owned prefixes, and a manual Windows exe
is never exec'd raw (the kernel's binfmt hands PEs to mono on the
reporter's box — observed hung process, 2026-10-10).

## Acceptance

- [x] auto: launcher-owned prefix → owner CLI (`bottles-cli run -b`, `lutris
  lutris:rungame/<slug>`, `heroic launch <app>`; flatpak argv forms);
  unroutable owner → launch error naming the missing piece; unowned →
  umu-run (error when disabled/not installed).
- [x] `umu` / `owner` route overrides, global + per-game
  (`LaunchRoute`, `LaunchRouteOverrides`), GUI dropdowns (settings +
  detail panel) and TUI cycle keys (settings `r`, detail `e`).
- [x] The umu toggle gates only the umu backend; routing works with it
  off. Raw PE exec removed from the manual path on Linux.
- [x] `go vet`, `go test ./...` green.

## Outcome

`umu.Prefix` gained `OwnerRef`/`Flatpak` (scan: bottle name, Lutris
slug from the config filename, Heroic app from GamesConfig
`winePrefix` mapping — closing the issue-48 gap); `LookupPrefix`
replaced `PrefixSource`. New `internal/launchercli` package: pure argv
builder + detached spawn (fake-CLI argv capture tests). The cmd hook
became the router (always wired on Linux, even without umu-run);
`ui.UmuLauncherHook` returns the backend name for the status line; the
ui gate dropped the UmuEnabled check so PE launches never reach the
raw-exec Launcher. Commit: THIS_COMMIT.
