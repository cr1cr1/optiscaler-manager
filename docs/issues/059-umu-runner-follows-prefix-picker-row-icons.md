---
title: Runner follows the prefix's launcher; picker rows show icon + launcher + prefix
description: umu launches now resolve PROTONPATH from the launcher owning the selected prefix, and prefix picker rows render "icon + Launcher: prefix".
status: done
---

# 059 — runner follows the prefix's launcher; picker rows show icon + launcher + prefix

## What and why

Two related umu-launcher gaps. (1) The runner was always the global
newest regardless of the chosen prefix, so a Bottles/Lutris/Heroic
prefix launched with e.g. Steam's newest Proton instead of the runner
its own launcher manages. (2) Picker rows read "name (source)", which
hides the launcher relationship; rows must read "icon + Launcher:
prefix" from the left.

## Acceptance

- [x] A pinned Bottles prefix launches with the newest runner from
  Bottles' runner root even when a newer Steam runner exists
  (cmd-level PROTONPATH capture test).
- [x] Launchers without runner roots (PlayOnLinux, CrossOver, ~/.wine,
  undetected/custom prefixes) and Auto fall back to the global newest,
  then umu's own runtime.
- [x] GUI rows render tick + launcher glyph (Typicons) + "Launcher:
  prefix"; TUI rows render a colored launcher initial + the same text.
- [x] `go vet`, `go test ./...` green.

## Outcome

`ResolveProtonPath(ctx, pinned, preferSource)` picks the newest runner
matching the prefix's source (runners are already source-tagged),
falling back to the global newest; the launch hook learns the source
via the new scan-based `PrefixSource(ctx, path)` (so even an
auto-resolved enclosing `drive_c` inside a bottle gets the Bottles
runner) and logs `prefix_source` on the resolved/failure lines.
`UmuPickerOption` now carries `Source` and `Name` separately with
`UmuPickerRowLabel` ("Bottles: mybottle") and `UmuSourceName`; GUI rows
map the source to a Typicons glyph (`umuSourceIcon`: Steam=anchor,
Bottles=wine glass, Lutris=puzzle, Heroic=flag, PlayOnLinux=play,
CrossOver=shuffle, Wine=beaker, Auto=sync, Custom=cog), TUI rows to a
colored initial (`umuSourceBadge`). Commit: ede9db3.
