---
title: Richer umu prefix detection (Steam names, Lutris, PlayOnLinux, CrossOver)
description: Steam compatdata prefixes are labeled with the game title from appmanifest, and prefix detection expands to Lutris YAML configs, PlayOnLinux, and CrossOver bottles.
status: done
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

Implemented as designed. Steam compatdata prefixes are labeled from
the sibling `appmanifest_<appid>.acf` (appid fallback); Lutris
prefixes come from per-game YAML `prefix:` paths (XDG config and
Flatpak, two-key line scanner, no YAML dependency); PlayOnLinux
`wineprefix/` and CrossOver `~/.cxoffice` children detected; all
read-only with the `drive_c` check. `go test ./...` passes for the
touched packages; the full suite at commit time also showed failures
from a parallel session's in-flight uninstall work (internal/ui,
internal/app — passing at the parent commit) and the pre-existing
sandbox-blocked `tmp/gpuprobe` probe. Committed as `4ca740d` — during
this work a parallel session raced the index and issue numbering
(drafted as 049, theirs claimed 049/050, hence 051); a foreign commit
the parallel uninstall work briefly landed under this change's message;
after both sessions finished, unpushed history was rebased so that work
reads correctly as `882c517` (issue 50). Open follow-up: none.
