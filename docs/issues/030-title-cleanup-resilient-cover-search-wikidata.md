---
title: Title cleanup, resilient cover search, Wikidata fallback
description: Generic title normalization for junk resolver output, punctuation-insensitive/token-subset Steam search with progressive query shortening, and a keyless Wikidata/Commons cover source.
---

# 030 — Title cleanup, resilient cover search, Wikidata fallback

## What and why

After issue 028 shipped, a rescan still left well-known games with junk
titles and placeholder art. Diagnosis against the live library:

- Titles are re-derived every scan, but the sources emit junk:
  Remedy's `Control.exe` is a launcher shim whose PE version info says
  "ControlLauncher"; goggame names carry subtitles ("Riven - The sequel
  to Myst"); folder titles leak paths and ancestor dir names
  ("WARDOGS (/mnt/linux3/...)", "Red Dead Redemption 2 (Games)") and
  repack tags ("... PROPER", "v1 0 10 0 MULTi13").
- The name→Steam cover bridge rejects anything whose result does not
  contain the full query verbatim: "Spelunky HD" vs "Spelunky",
  "Riven - The sequel to Myst" vs "Riven: The Sequel to MYST". Every
  rejection is negative-cached for 30 days (21 poisoned entries in the
  live cache), so rescans re-serve the miss without retrying.
- Non-PC games (a Cemu-hosted Zelda: Breath of the Wild) have no Steam
  or PCGamingWiki page, so no keyless source can find their art.

Fix generically, not per-title: clean resolver output, make the search
bridge tolerant, and add a keyless Wikidata/Commons cover fallback.
User approved this scope; SteamGridDB-key UI explicitly out of scope
(the settings key already works).

## Acceptance

- [ ] PE titles ending in a launcher-shim suffix ("ControlLauncher")
  fall through to the next resolver source; display titles are stripped
  of scene tags (PROPER, REPACK, MULTiNN) and trailing version runs
  ("v1 0 10 0"); duplicate-title disambiguation suffixes never leak
  absolute paths ("WARDOGS (/mnt/…)" → "WARDOGS (Wardogs/Wardogs)").
- [ ] Cover and identification lookups survive junk tokens: query
  variants (raw → gid-normalized → progressive right-truncation, ≥4
  chars) retry Steam storesearch, whose full-query substring matching
  otherwise answers zero items.
- [ ] Wikidata/Commons supplies cover art by title for games with no
  Steam/PCGW presence, scored with the shared gid matcher, normalized
  to the 2:3 invariant (issue 025), cached with negatives like the PCGW
  client.
- [ ] `go test ./...`, `go vet`, `gofmt`, and GOOS=windows/darwin
  builds all pass.

## Outcome

Shipped. Launcher-shim PE fallthrough + `cleanTitle` scene-tag/version
stripping in `discovery`; gid-normalized query variants in
`ui/identify.go`; raw → normalized → right-truncated query variants in
the cover chain's `searchAppID`; new keyless `internal/wikidata` client
(entity search → P18 via Commons, 30d cache with negatives) wired into
the cover chain after PCGW in both appid and name paths; duplicate-title
disambiguation now uses short path tails instead of absolute paths.
Diagnosis and live-API evidence are in docs/log.md (2026-10-08).
Commit: `5bc5e19`.
