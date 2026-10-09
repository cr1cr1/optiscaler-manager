---
title: Launcher-owned launch routing (umu vs owner CLIs)
description: Manual Windows games launch through the launcher that owns their prefix (bottles-cli, lutris, heroic), with umu-run only for unowned prefixes; raw PE exec is removed.
status: approved
---

# Launcher-owned launch routing — design

Date: 2026-10-10. Approved approach: A (launch router with per-owner CLI
backends), with the user's correction that routing is the primary launch
path for Windows binaries, not an umu-toggle add-on.

## Why

UMU/Proton mutates any prefix it launches against (runner DLL symlinks
into `system32`/`syswow64`, umu's `pfx/` compatibility layout). Running
a Bottles/Lutris/Heroic-owned prefix through umu mixes owners and can
corrupt the prefix for its real owner. Invariant: **one prefix = one
owner, and the owner launches it.**

Today this is violated twice: umu launches pinned foreign prefixes
(issues 48/51/53 mitigated runner choice, not ownership), and with umu
disabled a manual Windows exe is exec'd **raw** — on systems with the
mono binfmt registration (`/usr/lib/binfmt.d/mono.conf`: every MZ →
`/usr/bin/mono`) the game spawns mono and hangs (observed 2026-10-10).

## Scope

In: manual-store games whose exe is a Windows binary (PE MZ), plus the
Heroic-sourced Epic/GOG rows that are umu-eligible today — under `auto`
their Heroic-owned prefixes route to the heroic backend, consistent
with store delegation.
Out: Steam/Epic/GOG store rows (already delegate to their stores);
native Linux binaries (direct exec, unchanged); OptiScaler injection
(unaffected — DLLs live in the game dir regardless of who spawns it).

## Route decision

For a manual Windows-binary launch:

```
route = per-game override (LaunchRouteOverrides[canonicalDir])
      → global default (LaunchRoute)
      → "auto"
```

| route | behavior |
|---|---|
| `auto` | prefix (pin → global default → auto chain) has a scan-known owner → owner backend. Owner unroutable → launch error. Prefix unowned (app-managed `umu-prefixes/<slug>`, `~/.wine`, unknown) → umu-run if the umu toggle is on and `umu-run` is on PATH, else a launch error. |
| `umu` | always umu-run; error when umu disabled/not installed. |
| `owner` | always the owner backend; error when unroutable or unowned. |

The umu enable toggle shrinks to "umu is available as the backend for
unowned prefixes". Raw PE exec is removed from the manual path entirely.

Known v1 limitation (accepted): Lutris launches its configured game exe
(`lutris:rungame/<slug>`), not necessarily the exe optiscaler-manager
resolved. Bottles and Heroic take/behave per the exact exe.

## Prefix scan: owner references

`umu.Prefix` gains `OwnerRef string` and `Flatpak bool`, filled by the
existing `FindPrefixes` scan:

| source | OwnerRef | Flatpak |
|---|---|---|
| bottles | bottle name (= prefix dir name) | root under `~/.var/app/com.usebottles.bottles` |
| lutris | game slug (config filename stem, trailing `-<digits>` stripped) | root under `~/.var/app/net.lutris.Lutris` |
| heroic | app name via `GamesConfig/*.json` `winePrefix` mapping (closes the issue-48 gap); fallback: prefix dir name | root under `~/.var/app/com.heroicgameslauncher.hgl` |
| steam, playonlinux, crossover | "" (unroutable by design) | — |
| wine (`~/.wine`), unknown | "" → umu-eligible | — |

## Backends — new `internal/launchercli` package (build-tagged linux/!linux)

Each backend: `Detect() (argv0 []string, ok bool)` + command builder;
spawned detached via the existing spawner pattern (Start + Release,
never Wait); flatpak installs swap the argv head.

- **bottles**: `bottles-cli run -b <ref> -e <exe>` · flatpak:
  `flatpak run --command=bottles-cli com.usebottles.bottles run …`
- **lutris**: `lutris lutris:rungame/<ref>` · flatpak:
  `flatpak run net.lutris.Lutris lutris:rungame/<ref>`
- **heroic**: `heroic launch <ref>` · flatpak:
  `flatpak run com.heroicgameslauncher.hgl launch <ref>`
  (only when the GamesConfig mapping produced a ref)

Detection is `exec.LookPath` plus flatpak app presence
(`~/.var/app/<id>` dir exists); no version probing.

## Settings schema & UI

- `settings`: `LaunchRoute string` (`""`=auto, `umu`, `owner`) +
  `LaunchRouteOverrides map[string]string` keyed by canonical install
  dir — same persistence pattern as `PrefixOverrides`.
- GUI: global 3-option dropdown in the settings umu tab beside the
  prefix picker; per-game "Launch via" dropdown in the detail panel
  beside the prefix dropdown. Shared dropdown machinery.
- TUI: settings row cycles the global value; a detail-screen key cycles
  the per-game value Auto→umu→owner.

## Errors, logging

Unroutable launches return errors through the existing failure surface
(status line + log panel, issue 52), naming owner, missing piece, and
remedy: "bottles-cli not on PATH — install Bottles or set Launch via:
umu", "no Lutris game maps to this prefix", "compatdata belongs to
Steam — launch the Steam row", "Windows exe with umu off and no
launcher-owned prefix". Resolved launches log `route`, `owner`,
`owner_ref` beside the existing `game/exe/prefix/proton`; owner-backend
failures keep the stderr-tail wrapping.

## Testing

- `internal/umu`: owner-ref scan tests (bottle name, lutris slug strip,
  heroic GamesConfig mapping, flatpak flag), linux-tagged.
- `internal/launchercli`: fake `bottles-cli`/`lutris`/`heroic` on PATH
  capturing argv (fake-`umu-run` pattern), flatpak argv shapes,
  missing-CLI errors.
- `cmd`: route matrix — auto/umu/owner × owned/unowned/unroutable —
  via fake CLIs; PE-with-umu-off asserts the error, not a spawn.
- `internal/ui`: LaunchRoute persistence, per-game override precedence
  (mirrors prefix-pin tests).
- GUI/TUI: dropdown render + persist (seam tests, issue 48/56 pattern).
- Real-system smoke: Bottles bottle pin → `bottles-cli` argv correct
  (Bottles 67.4 native on the dev box; Lutris/Heroic not installed —
  their backends are fixture-tested only).
