---
title: Per-game umu wine-prefix pinning and launch failure diagnostics
description: The settings runner picker is replaced by prefix detection/pinning (global default plus per-game pins), and umu launch failures log full context (game, binary, prefix, runner, stderr tail).
status: done
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

Implemented as designed, commit `20a18a5`. `umu.FindPrefixes` detects
prefixes across Heroic (native + Flatpak), Bottles (XDG + Flatpak),
Steam compatdata `pfx` (all roots and extra libraries), and `~/.wine`;
Lutris prefixes remain covered by the enclosing-`drive_c` auto rule.
Launch-time precedence: per-game `PrefixOverrides` → global
`UmuPrefixPath` → auto chain, read live via a late-bound
`sess.UmuPrefixFor` resolver in the launch hook. Proton is always Auto;
`umu_proton_path` is retired (legacy JSON loads ignored). GUI settings
picker + detail-panel prefix dropdown (Linux-gated via
`Session.UmuPlatform`), TUI `p` (global) and `w` (per-game) pickers,
rows composed once in `ui.UmuPickerRows` fed by `Deps.UmuPrefixes`.
Failures carry umu's stderr tail (last ≤10 non-empty lines, ≤1 KiB) and
the hook logs game/exe/prefix/proton. `go test ./...` passes except the
pre-existing `tmp/gpuprobe` failure (needs `/dev/dri`, fails identically
on the parent commit); `go vet ./...` clean; darwin/windows
cross-compile OK. Open follow-up: Lutris prefixes could be listed by
parsing per-game YAML configs if users ask for it.
