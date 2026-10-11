---
title: Runners list includes Steam's official Proton builds from steamapps/common
description: The runner picker only scanned compatibilitytools.d (custom tools like GE-Proton/UMU-Proton); Valve's official Proton builds install as apps under steamapps/common in the Steam root and every extra library, so they never appeared — they are now scanned with a Proton-name + proton-script gate.
---

# 075 — Runners list includes Steam's official Proton builds

## What and why

User request: the Runners list should include all Steam runners found in
the well-known Steam locations and, where present, the extra Steam
libraries (which the config already extracts via libraryfolders.vdf).

Root cause of their absence: `RunnerRoots` scans only
`compatibilitytools.d` directories (custom tools: GE-Proton, UMU-Proton)
across Steam roots and libraries. Valve's official Proton builds are
Steam *apps* and install under `<root-or-library>/steamapps/common/`
(verified on the user's machine: `Proton - Experimental` there ships the
top-level `proton` script umu's PROTONPATH needs), so they never surfaced
in the settings runner picker, the per-game override, or
`ResolveProtonPath`'s auto chain.

Fix: add every Steam root/library's `steamapps/common` to the runner
roots, gated in `FindRunners` to entries whose name starts with "proton"
*and* that ship the top-level `proton` script. The gate excludes games,
`Steamworks Shared`, and the `SteamLinuxRuntime*` containers (which carry
`toolmanifest.vdf` and would otherwise pass `runnerValid`), as well as
the Proton EasyAntiCheat/BattlEye runtimes (no `proton` script). Source
labels classify `steamapps/common` as "steam".

## Acceptance

- [x] `RunnerRoots` includes `<root>/steamapps/common` for each well-known
      Steam location and each extra library from libraryfolders.vdf
      (existing dirs only, symlink-deduped as before).
- [x] A `steamapps/common` child named `Proton …` with a top-level
      `proton` script appears in `FindRunners` with Source "steam".
- [x] Games, `SteamLinuxRuntime*` (toolmanifest-bearing), and
      `Proton EasyAntiCheat Runtime` (no proton script) do not appear.
- [x] Non-Steam roots keep the existing `runnerValid` behavior.
- [x] `go test ./...`, `go vet`, `gofmt` green; docs updated.

## Outcome

Implemented as designed. Red witnessed: `FindRunners` over a synthetic
steamapps/common accepted all four dirs (both Protons plus the EAC
runtime and SteamLinuxRuntime_sniper, both toolmanifest-bearing) with
Source "other"; `RunnerRoots` lacked the common roots.

- `internal/umu/runners_linux.go`: `steamLibraryCommon` maps every Steam
  root/library to its `steamapps/common`; `RunnerRoots` appends them
  (existing-dirs and symlink-dedup rules unchanged); `FindRunners` gates
  those roots with `steamCommonRunner` (proton* name prefix + top-level
  `proton` script); `rootSource` classifies `steamapps/common` as
  "steam". All three consumers (`Session.UmuRunners` dropdown,
  `ResolveProtonPath` auto chain) pick the new runners up unchanged.
- Ground truth on the user's machine: `Proton - Experimental` from the
  extra library's steamapps/common now lists with Source "steam",
  sorting after versioned builds (versionless → "0"); sniper/EAC absent.
- Flake note: one unrelated `internal/ui` run failed on
  `TestClearCoverOverrideRestoresChain` TempDir cleanup ("directory not
  empty" — async games-cache write racing teardown); the rerun was
  35/35 green. Pre-existing race, not touched here.
- Docs: scope (umu bullet runner enumeration).

Commit: 1c40733
