---
title: Steam prefixes named from appinfo.vdf when no appmanifest exists
description: Steam compatdata prefixes whose game is uninstalled (or installed in another library) had no sibling appmanifest, so the settings/per-game prefix dropdowns listed the raw appid instead of the game's display name; FindPrefixes now falls back to Steam's local appcache/appinfo.vdf.
---

# 074 — Steam prefixes named from appinfo.vdf when no appmanifest exists

## What and why

User report: the prefix dropdowns (settings and per-game) list some Steam
prefixes by their internal appid instead of the game's display name.

Root cause (verified on the user's machine): `FindPrefixes` names a Steam
compatdata prefix from `appmanifest_<appid>.acf` in the *same* steamapps
dir. When the game is uninstalled (compatdata left behind) the manifest is
gone from every library, so the name falls back to the raw directory name
— the appid. All five orphaned compatdata dirs on the machine
(312520, 578080, 1449560, 2180100, 2805730) resolve to real display names
via Steam's local `appcache/appinfo.vdf` (current `0x07564429` string-pool
layout): Rain World, PUBG: BATTLEGROUNDS, Metro Exodus Enhanced Edition,
Proton Hotfix, Proton 9.0.

Fix: parse `appcache/appinfo.vdf` (legacy `0x07564427` and string-pool
`0x07564428/29` layouts) from each Steam install root — native, XDG,
Flatpak, Snap — into an appid → name map, and use it as the fallback when
no appmanifest names a Steam compatdata prefix. Offline, no API, one parse
per FindPrefixes call and only when a manifest lookup misses. Unresolvable
ids (appid 0 shared prefix, >2³¹ non-Steam shortcut ids) keep the raw id.

## Acceptance

- [ ] A compatdata prefix with no appmanifest but an appinfo.vdf entry is
      listed under the game's display name in the prefix dropdowns
      (settings picker and per-game picker share `UmuPickerRowLabel`).
- [ ] An appmanifest name still wins over appinfo.vdf when both exist.
- [ ] A compatdata prefix with neither keeps the raw appid; a missing or
      corrupt appinfo.vdf changes nothing (no error surfaced).
- [ ] Both the legacy and string-pool appinfo.vdf layouts parse.
- [ ] `go test ./...`, `go vet`, `gofmt` green; docs updated.

## Outcome

Implemented as designed. Reds witnessed before the fix: both parser tests
read `map[]` and the FindPrefixes fallback test saw `Name = "312520"`. One
implementation bug caught by the two-record test: the record walker
double-advanced past each record (parseMap had already consumed the body),
so only single-record files parsed — fixed by snapping the cursor to the
record end.

- `internal/umu/appinfo_linux.go` (new): `appinfoNames` parses
  appcache/appinfo.vdf — legacy `0x07564427` (cstring keys, single SHA)
  and string-pool `0x07564428/29` (uint32 pool-index keys, dual SHA)
  layouts — into appid → name; nil on missing/unknown/corrupt content.
- `internal/umu/runners_linux.go`: the install-root candidates factored
  out of `steamLibraryRoots` into `steamInstallDirs` (appcache lives in
  the install root, not in extra library folders).
- `internal/umu/prefixes_linux.go`: on an appmanifest miss the steam
  compatdata name falls back to the appinfo map, parsed lazily once per
  FindPrefixes call on the first miss; the raw appid stays when neither
  source knows the id (appid 0 shared prefix, >2³¹ non-Steam shortcut
  ids).
- Verified against the machine's real 6.4 MB appinfo.vdf (2445 records,
  ~20 ms): all five orphaned compatdata dirs resolve — 312520 → Rain
  World, 578080 → PUBG: BATTLEGROUNDS, 1449560 → Metro Exodus Enhanced
  Edition, 2180100 → Proton Hotfix, 2805730 → Proton 9.0. Both dropdowns
  (settings picker, per-game picker) and the TUI picker share
  `UmuPickerRowLabel`, so the source fix covers all three.
- Docs: scope (umu bullet naming chain).

Commit: THIS_COMMIT
