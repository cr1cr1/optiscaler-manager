---
title: umu-launcher runtime and prefix autodetection
description: Launching via umu must work with the Proton path setting left empty, by auto-detecting Proton runners and existing wine prefixes from Steam, Bottles, Lutris, Heroic, and well-known umu locations.
---

# 047 — umu-launcher runtime and prefix autodetection

## What and why

Launching a game through umu-launcher breaks when the bespoke
`umu_proton_path` setting holds a wrong path, and leaving it empty used to
mean "umu resolves its own default" even when the user already has Proton
builds installed via Steam, Bottles, Lutris, or Heroic. The setting now
defaults to empty (Auto): the app detects Proton runners across the
well-known launcher roots (native, Flatpak, Snap, extra Steam libraries)
and picks the newest at launch; the manual path remains as an override,
now chosen from a picker of detected runners instead of free text. Wine
prefixes are detected too: a game whose exe already lives inside a
launcher prefix (Bottles/Lutris/Heroic `drive_c`) or that has an existing
Heroic prefix reuses it; anything else keeps the app-managed per-install
prefix. Nothing is created or migrated inside launcher directories.

## Acceptance

- [ ] `RunnerRoots` finds Steam (native/Flatpak/Snap/extra libraries via
  `libraryfolders.vdf`), Bottles, Lutris, Heroic, and umu runner roots,
  existing dirs only, symlink-deduped.
- [ ] A runner dir is valid with `toolmanifest.vdf` OR a top-level
  `proton` script; `bin/wine`-only builds are skipped.
- [ ] `ResolveProtonPath`: pinned path wins; empty picks the newest
  detected runner; no runners yields "" (umu's own runtime).
- [ ] Prefix resolution reuses an existing enclosing `drive_c` prefix,
  then an existing Heroic prefix for the game's AppName, else the
  app-managed prefix.
- [ ] GUI and TUI settings offer a picker ("Auto (newest detected)" +
  detected runners + a Custom row for an unknown persisted path).
- [ ] `go test ./...` and `go vet ./...` pass.

## Outcome

Filled in when the issue closes: what actually happened, the commit
hash, anything left open.
