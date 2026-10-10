---
title: Per-game launch env map (DXVK_*/PROTON_* passthrough)
description: Per-game extra env vars (env_overrides) flow into umu launches as ExtraEnv; editable in the TUI detail screen (E) and the GUI detail panel (inline editor), empty clears.
status: done
---

# 067 — per-game launch env map

## What and why

From the umu design notes: "Other env vars (DXVK_*, PROTON_*,
WINEDLLOVERRIDES, etc.) pass through to Proton. Treat the per-game
env map as an open dictionary." `LaunchOpts.ExtraEnv` existed but
nothing could set it per game.

## Acceptance

- [x] Settings `env_overrides`: per canonical install dir → open
  dictionary; persisted; `UmuEnvFor` (copy) / `SetUmuEnvOverride`
  (empty clears; unknown game refuses).
- [x] Edit format `KEY=VALUE KEY=VALUE` — `ParseEnv` (errors name the
  bad token) / `FormatEnv` (sorted, stable).
- [x] The router passes the pin as `ExtraEnv` (cmd test: child env
  carries the pairs).
- [x] TUI: `E` on the detail screen edits the pin (pre-filled; invalid
  text toasts and keeps editing; empty clears).
- [x] GUI: detail panel "Env (n)" button → inline editor with
  Apply/Cancel, Enter/Esc hooks, panel-switch cancel — the title
  editor's discipline.
- [x] `go vet`, `go test ./...` green; darwin/windows cross-compile OK.

## Outcome

`newUmuLauncher` grew to seven positional params — the DEBT comment
(flagged since issue 064) now says the next growth turns it into a
resolver struct. Commit: aab6aec.
