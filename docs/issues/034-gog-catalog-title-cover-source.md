---
title: GOG catalog as a title and cover source
description: Keyless GOG catalog API (catalog.gog.com) feeds identification (canonical titles) and the cover chain (vertical store art), reusing the gid scorer and the numeral rule.
---

# 034 — GOG catalog as a title and cover source

## What and why

User request: add gog.com as a game database for both matches and
covers. The keyless Galaxy catalog API
(`catalog.gog.com/v1/catalog?query=like:<term>`) returns product titles
plus `coverVertical` portrait art (verified live: Witcher 3, Control,
Riven (1997) all resolve; 342x482 art, cropped to 2:3 by the existing
invariant). Its `like:` ranking degrades with junk tokens exactly like
Steam's storesearch, so the issue-030 query variants and the issue-033
numeral veto/corroboration apply unchanged.

- Identification (`ui/identify.go`): after Steam and PCGW, GOG titles
  are scored with `gid.BestAccepted` (tertiary canonical source).
- Covers (`internal/covers`): after PCGW, before Wikidata — GOG art is
  store-quality vertical covers; Wikidata stays the last structured
  resort. The Steam item binding and the GOG product binding share one
  `bindCandidate` helper (gid score + numeral rule).

## Acceptance

- [ ] New `internal/gogdb` client: keyless catalog search, games only,
  paced, 429/5xx cooldown, 30d disk cache with negatives (mirrors the
  pcgw/wikidata discipline).
- [ ] Identification canonicalizes titles via GOG when Steam and PCGW
  find nothing.
- [ ] Cover chain binds GOG `coverVertical` art under the numeral rule
  (Witcher 1 vetoed for a Witcher 3 title; Riven's 1997 original wins
  over the remake), beats Wikidata, and loses to PCGW.
- [ ] `go test ./...`, `go vet`, `gofmt`, GOOS=windows/darwin builds
  pass.

## Outcome

Shipped. New `internal/gogdb` client (keyless Galaxy catalog search,
games-only filter, 500ms pacing, 429/5xx cooldown, 30d disk cache with
negatives — keyed by the RAW term after a debug session showed
normalized keys let a junky raw query's negative poison its productive
normalized variant). Covers: `fromGOG` walks the issue-030 query
variants and binds via the new shared `bindCandidate` (gid scorer +
issue-033 numeral rule, now index-returning and shared by Steam and GOG
so the rule cannot drift; sentinel fixed — a penalized corroborated
score went below the -1 sentinel). Chain: … → PCGW → GOG → Wikidata in
both appid and name paths. Identification: GOG titles are the tertiary
canonical source in `ui/identify.go` after Steam/PCGW. Red witnessed
(`tmp/test-red-034.log`, compile reds on all three seams); two real
bugs caught by the green run and fixed (cache-key poisoning, corr
sentinel). Full `go test ./...` exit 0 (30 packages), vet/gofmt clean,
windows/darwin builds OK. Commit: pinned after the fact.
