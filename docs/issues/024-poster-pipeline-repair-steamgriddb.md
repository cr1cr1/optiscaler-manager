---
title: Repair the poster pipeline (dead PCGW Cargo API) and add SteamGridDB as an art source
description: PCGamingWiki's Cargo API now rejects anonymous queries and the client caches those errors as 30-day negatives, killing the non-Steam cover fallback; replace Cargo with wikitext/imageinfo calls, detect API error payloads, invalidate the poisoned cache, and add SteamGridDB (user API key) as a cover source for games Steam's CDN has no art for.
---

# 024 — Poster pipeline repair + SteamGridDB art source

## What and why

User report: well-known games show no posters — "The Witcher 3
Remastered" and "End of Abyss" (both manually added via scan dir), and
"Wardogs" (a Steam game). Live probing found three independent root
causes:

1. **PCGW's Cargo API is dead for anonymous clients.** `CoverFile` and
   `TitleBySteamAppID` query `action=cargoquery`, which now returns HTTP
   200 with `{"error":{"code":"permissiondenied"}}` (SMW was retired in
   2022; `insource:` search indexes nothing — there is no anonymous
   replacement for the appid→page reverse lookup). `get()` only checks
   the HTTP status, decodes the error body as "empty results", and
   **caches it as a 30-day negative** — the fallback is not just broken
   but self-poisoning (~110 poisoned cache files observed on the
   reporter's machine). The anonymous endpoints we need all still work:
   `opensearch` (title search), `prop=revisions` wikitext (the infobox
   `|cover = X` field), and `prop=imageinfo` thumbnails.
2. **"Wardogs" (appid 1867240) has no art on any Steam CDN host** —
   `library_600x900.jpg`, `library_hero.jpg`, `header.jpg` all 404 on
   both the legacy and the new `store_item_assets` paths. No keyless
   source can fix this; SteamGridDB carries community/official grids for
   such games.
3. **"The Witcher 3 Remastered"** drops the "Wild Hunt" subtitle, so the
   deliberately strict scorer (issue: "a wrong cover is worse than no
   cover"; "Frostpunk" must not bind "Frostpunk 2") rejects the correct
   Steam and PCGW matches. SteamGridDB's alias-aware autocomplete
   resolves such names; its top hit is trusted with a scoped,
   cover-only acceptance rule (below) — the global scorer is NOT
   loosened, and identify/title binding is untouched.

Two slices:

### A. PCGW repair (keyless)

- **`get()` detects API error payloads.** A MediaWiki `{"error":{...}}`
  envelope (HTTP 200) becomes a live error; callers already cache
  negatives only on *empty results*, so API errors stop poisoning the
  cache.
- **`CoverFile` via wikitext.** `action=query&prop=revisions&
  rvprop=content&rvslots=main&titles=<page>`, parse the `|cover = X`
  infobox line (case-insensitive key, surrounding whitespace). Replaces
  the cargoquery.
- **`SearchTitle` → `SearchTitles`** returning every opensearch hit
  (not just the first); both callers (`covers.fromPCGW`,
  `ui.identifyRow`) score all candidates and bind the best accepted
  one. Today an unacceptable first hit buries an exact-match second
  hit.
- **`TitleBySteamAppID` deleted** — no anonymous replacement exists and
  its only production caller is `fromPCGW`'s appid shortcut; title
  search covers the same ground.
- **Cache invalidation (clean break).** The cache-key hash input gains
  a `v2:` prefix so every pre-fix entry (poisoned or not) is orphaned
  and unread. Legacy `~/.cache/optiscaler-manager/pcgw/*.json` files
  can be deleted by hand; the code never reads them again.

### B. SteamGridDB art source

- New `internal/sgdb` client: `https://www.steamgriddb.com/api/v2` with
  `Authorization: Bearer <key>`. Endpoints: `GET /games/steam/{appid}`
  (appid → game), `GET /search/autocomplete/{term}` (name → game, top
  hit), `GET /grids/game/{id}?dimensions=600x900&types=static` (first —
  score-sorted — grid URL). Same discipline as the pcgw client: paced
  requests, descriptive UA, 30d disk cache with negatives, `success:
  false` bodies are live errors and never cached.
- `settings.Settings.SteamGridDBKey` — **JSON-edited only** (the
  TitleOverrides precedent); empty means SGDB silently disabled.
  README documents where to create the free key.
- `covers.Covers.SGDB *sgdb.Client` (nil → skipped), wired in
  `cmd/session.go` when a key is configured. Chain positions (portrait
  official art still wins; SGDB before the wiki and the landscape hero):
  - appid path: CDN 600x900 → **SGDB by appid** → PCGW by title → CDN
    hero → miss marker.
  - name path: Steam search → CDN (unchanged) → **SGDB by name** →
    PCGW (unchanged).
- **SGDB name acceptance (scoped):** the top autocomplete hit binds when
  `gid.Accept(gid.Score(...))` passes, OR when the candidate's normalized
  tokens are a subset of the hit's and the hit adds no new numeral
  tokens ("the witcher 3" ⊆ "the witcher 3 wild hunt" ✓; "frostpunk" ⊄
  "frostpunk 2" ✗ — new numeral). This rule exists ONLY for picking
  decorative cover art from SGDB's alias-aware search; it does not touch
  `identifyRow`, SteamAppID binding, or the Steam/PCGW paths.

## Acceptance

- [x] pcgw: error-envelope test — fake server returns 200 +
  `{"error":{"code":"permissiondenied"}}` → callers get a live error,
  nothing is cached (second call hits the server again).
- [x] pcgw: `CoverFile` parses `|cover = X` from wikitext (fake server),
  caches positives and true-empty negatives.
- [x] pcgw: `SearchTitles` returns all hits; `fromPCGW` binds a later,
  acceptable hit when the first is rejected.
- [x] pcgw: legacy (pre-`v2:`) cache files are ignored.
- [x] pcgw: `TitleBySteamAppID` and its tests are gone; `go vet` clean.
- [x] sgdb: client tests with a fake server — appid lookup, autocomplete,
  grid URL, `success:false` → live error, 404 → `ErrNoMatch`, caching.
- [x] covers: CDN 404 → SGDB-by-appid grid is downloaded and cached;
  SGDB nil → behavior unchanged.
- [x] covers: name path — "The Witcher 3 Remastered"-style candidate
  binds SGDB's "The Witcher 3: Wild Hunt" top hit (subset rule);
  "Frostpunk" does NOT bind "Frostpunk 2" (numeral guard).
- [x] settings: `steamgriddb_key` round-trips Load/Save; legacy files
  without the key load as "".
- [x] Full `go test ./...` green, `go vet`/`gofmt` clean,
  `GOOS=windows`/`darwin go build ./...` OK.
- [x] Docs: this issue, `docs/log.md`, `docs/architecture.md` (covers
  chain, pcgw module, new sgdb module), `docs/scope.md`, `README.md`
  (SGDB key setup; note the title-override workaround for
  subtitle-mismatch names).

## Outcome

Implemented as specced, with two refinements found while testing:

- The wikitext cover regex initially used `\s*` around the value, which
  let an empty `|cover = ` line swallow the NEXT infobox line (`\s`
  matches `\n`); only horizontal whitespace is allowed now (caught by
  the true-negative unit test).
- `settings.Load` decodes through an explicit raw struct, so the new
  `steamgriddb_key` field needed plumbing in two places (round-trip
  test caught the drop).
- The spec promised pcgw-style request pacing for SGDB; it was dropped
  as unneeded — cover resolution is inherently sequential per scan and
  the client caches aggressively, so bursts cannot occur.

`gid.BestAccepted` (highest-scoring acceptable candidate) is the single
owner of "score all, pick best" for both `covers.fromPCGW` and
`ui.identifyRow`. The SGDB name gate (`sgdbNameAccepts`) lives in
`covers` and is covers-only by construction. The repaired PCGW chain was
verified live end to end for "End of Abyss": opensearch → wikitext
`|cover = End of Abyss cover.jpg` → imageinfo → 600×900 JPEG downloads.
"Wardogs" (appid 1867240) has no art on any Steam CDN host (all
variants, both legacy and `store_item_assets` paths, 404) and needs the
SGDB key path, as does the subtitle-dropping "The Witcher 3 Remastered"
folder name (or a `title_overrides` pin, now documented in the README).

ATDD reds witnessed per slice (Slice A: pcgw/gid compile red + covers
fake-server failures; Slice B: sgdb/covers/settings compile red).
Commit: _pinned in the follow-up commit_. Verification:
`go test ./...` (30 packages) exit 0, `go vet`/`gofmt` clean,
`GOOS=windows`/`darwin go build ./...` OK.
