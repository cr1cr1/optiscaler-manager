---
title: Richer umu prefix detection (Steam names, Lutris, PlayOnLinux, CrossOver)
description: Steam compatdata prefixes are labeled with the game title from appmanifest, and prefix detection expands to Lutris YAML configs, PlayOnLinux, and CrossOver bottles.
---

# 051 — richer umu prefix detection

## What and why

The prefix picker lists Steam compatdata prefixes as bare appids
(`12345 (steam)`), which is unreadable, and covers only Steam, Heroic,
Bottles, and `~/.wine`. Steam prefixes are now labeled with the game
title parsed from the sibling `appmanifest_<appid>.acf` (falling back
to the appid), and detection expands to Lutris (per-game `prefix:`
paths from `~/.config/lutris/games/*.yml`, native and Flatpak),
PlayOnLinux (`~/.PlayOnLinux/wineprefix`), and CrossOver
(`~/.cxoffice`). Everything stays read-only with the `drive_c`
validity check.

## Acceptance

- [ ] Steam prefixes are named from `appmanifest_<appid>.acf` when
  present, appid otherwise.
- [ ] Lutris prefixes come from the per-game YAML configs (name from
  the config's `name:`, path from `game: prefix:`), native and
  Flatpak, XDG_CONFIG_HOME honored.
- [ ] PlayOnLinux and CrossOver bottles are detected.
- [ ] `go test ./...` and `go vet ./...` pass.

## Outcome

Filled in when the issue closes: what actually happened, the commit
hash, anything left open.
