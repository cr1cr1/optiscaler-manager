---
title: Heroic Epic/GOG game scanning on Linux
description: On Linux, discover Epic and GOG games installed via Heroic (native + Flatpak) from its installed.json files as first-class StoreEpic/StoreGOG rows, and let them launch through umu like manual games.
issue: 12
status: done
---

# 12 — Heroic Epic/GOG game scanning on Linux

## What and why

On Linux the Epic and GOG storefront probes are stubbed out, so games
installed through Heroic Games Launcher are invisible unless the user
hand-adds install directories as recursive roots. Heroic records every
installed game in plain JSON (`legendary/installed.json` for Epic,
`gog_store/installed.json` for GOG) under both its native and Flatpak
config dirs — parsing those gives first-class `StoreEpic`/`StoreGOG`
rows with no new dependencies. Windows and macOS behavior is unchanged
(their native probes already work).

Design (brainstormed and approved 2026-07-22): a stdlib
`gid.ParseHeroicInstalled` parser; a Linux-only `discovery.heroicGames`
probing `$XDG_CONFIG_HOME`/`~/.config`/`~/.var/app/.../config` Heroic
roots, wired into `ScanAll` between the Epic and GOG probes (existing
canonical-dir dedupe resolves overlaps); `shouldUseUmu` extended to
Epic/GOG rows with an ExePath so the games actually launch on Linux.

## Acceptance

- [x] `gid.ParseHeroicInstalled` parses Epic and GOG installed.json
      shapes; broken JSON and missing required fields error/skip.
- [x] Linux probe finds native + Flatpak Heroic roots, skips entries
      with missing install dirs and non-Windows platforms, resolves
      ExePath only when the executable exists.
- [x] `ScanAll` returns Heroic Epic/GOG rows; a recursive-root duplicate
      of the same install dir loses to the Heroic row.
- [x] `shouldUseUmu` is true for Linux Epic/GOG rows with an ExePath,
      false on Windows and for rows without one.
- [x] `go test ./...` green; docs updated.

## Outcome

- `internal/gid/heroic.go`: `HeroicEntry` + `ParseHeroicInstalled` —
  stdlib-only, accepts legendary's snake_case and the GOG store's
  camelCase `app_name`/`appName`, backfills app name from the map key
  and title from the app name, drops entries with no install path,
  sorted output; `IsWindows()` treats an empty platform as Windows
  (older legendary files).
- `internal/discovery/heroic_linux.go` (+ `heroic_stub.go`): probes
  `$XDG_CONFIG_HOME/heroic` (or `~/.config/heroic`) and the Flatpak
  config root for `legendaryConfig/legendary/installed.json` (Epic) and
  `gog_store/installed.json` (GOG); skips non-Windows builds and missing
  install dirs; ExePath from the record's executable (separator
  normalization + `joinWithin` escape rejection) with the goggame-info
  `GOGExePath` fallback for GOG records. One `add(heroicGames())` line
  in `ScanAll`, between the Epic and GOG probes.
- `internal/ui/launch.go`: `shouldUseUmu` now admits Epic/GOG rows
  (Linux-only, UmuEnabled, Windows-binary ExePath still required) so
  Heroic games launch through umu-run like manual games instead of the
  best-effort store URL / direct PE exec. Doc comments updated across
  session/settings/umu/cmd.
- Tests (TDD, reds witnessed): gid parser fixtures (both shapes,
  fallbacks, broken JSON), heroic probe (native + Flatpak roots,
  skip rules, exe escape rejection, broken file doesn't block others),
  ScanAll merge with dedupe, four session launch tests (Epic/GOG umu,
  no-exe fallback, Windows opt-out). Fixtures write 4-byte MZ magic
  (isBinaryMagic ReadAt()s 4 bytes).
- `go test ./...` green, `go vet`/`gofmt` clean, GOOS=windows/darwin
  builds green. README + docs/architecture.md updated.
  ponytail-review folded in (one note deleted). (b171a97)
