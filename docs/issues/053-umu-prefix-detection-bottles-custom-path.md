---
title: umu prefix detection honors Bottles custom_bottles_path
description: Bottles prefixes configured with a custom bottles directory were invisible to the prefix picker; detection now reads data.yml for the custom root.
status: done
---

# 053 — umu prefix detection honors Bottles custom_bottles_path

## What and why

`FindPrefixes` only probed Bottles' default roots (XDG
`bottles/bottles` and the Flatpak variant). Users who set a custom
bottles directory in Bottles saw no Bottles prefixes in the picker at
all. Bottles records that choice in `data.yml` (next to the default
root, native or Flatpak) as `custom_bottles_path: <dir>`, with the
prefixes living directly under `<dir>`; detection now reads both
`data.yml` locations and scans the custom root as another `bottles`
container.

## Acceptance

- [x] A prefix under the root named by `custom_bottles_path` in
  `~/.local/share/bottles/data.yml` is detected with source `bottles`.
- [x] Default-root detection, `go vet`, and `go test ./...` stay green.

## Outcome

`bottlesCustomPath` parses the `custom_bottles_path:` key (quotes
stripped via the existing `unquoteYAML`) from the native and Flatpak
`data.yml`; a non-empty value becomes a `prefixContainer{root,
"bottles"}`. Missing files/keys and non-prefix children (e.g.
Bottles' `_users` dir) are skipped by the existing container walk and
`drive_c` check. Verified against the reporter's real layout
(`/mnt/linux1/bottles` with `proton11`/`win-apps`/`wine11` prefixes).
Commit: 478ef7a.
