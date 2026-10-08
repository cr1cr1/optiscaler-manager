---
title: Per-game umu wine-prefix pinning and launch failure diagnostics
description: The settings runner picker is replaced by prefix detection/pinning (global default plus per-game pins), and umu launch failures log full context (game, binary, prefix, runner, stderr tail).
---

# 048 — per-game umu wine-prefix pinning + launch failure diagnostics

## What and why

Issue 47 added runner detection, but what users actually need to control
is the wine *prefix*: which existing bottle a game launches in. The
settings Proton-runner picker is replaced by a prefix picker fed by
detected wine prefixes (Heroic `~/Games/Heroic/Prefixes`, Bottles
`bottles/`, Steam `steamapps/compatdata/<appid>/pfx`, `~/.wine`). The
prefix can be pinned globally (Settings, the default for all games) and
per game (detail pane / detail screen), per-game winning over global,
both falling back to the issue-47 auto chain when empty. Proton is now
always Auto (newest detected runner); the `umu_proton_path` pin is
retired (legacy JSON keys are ignored). Separately, umu launch failures
were nearly invisible: errors now carry umu's stderr tail and the hook
logs game title, exe path, prefix, and runner on failure.

## Acceptance

- [ ] `umu.FindPrefixes` detects prefixes (dirs containing `drive_c`)
  across Heroic, Bottles, Steam compatdata (`pfx`), and `~/.wine`;
  Flatpak variants included.
- [ ] Resolution order: per-game pin → global `UmuPrefixPath` → auto
  chain (enclosing `drive_c` → Heroic → app-managed).
- [ ] `UmuProtonPath` setting and runner picker removed; legacy
  `umu_proton_path` JSON loads without error.
- [ ] GUI settings picker and TUI `p` set the global default; GUI detail
  panel and TUI detail screen pin per game (Linux only).
- [ ] Launch failures log game/exe/prefix/proton and errors carry the
  umu stderr tail.
- [ ] `go test ./...` and `go vet ./...` pass.

## Outcome

Filled in when the issue closes: what actually happened, the commit
hash, anything left open.
