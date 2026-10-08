---
title: Numeral guard for cover search binding
description: Truncated franchise-root queries must not bind items whose digit tokens differ from the original title's ("The Witcher 3…" binding Witcher 1 art).
---

# 033 — Numeral guard for cover search binding

## What and why

Issue 030's progressive query truncation introduced a franchise-root
trap: "The Witcher 3: Wild Hunt - Game of the Year Edition" truncates
to "the witcher" (the ® and colon in Steam's name defeat substring
matching at every longer variant), and edition-token stripping makes
"The Witcher: Enhanced Edition Director's Cut" normalize to exactly
"the witcher" — an exact match that bound Witcher 1's poster (20900)
to a Witcher 3 row. The identifying numeral was truncated away and
nothing required it to match.

Fix generically: digit tokens of the ORIGINAL title govern binding —
mismatched items are refused outright ("The Witcher 3 …" rejects
"The Witcher: Enhanced Edition"), and a non-empty MATCHING set
corroborates a weak truncated query whose tokens subset the item's
("the witcher" + {3} binds Witcher 3's real art; "doom" never binds
Doom Eternal). A wrong cover is worse than no cover; the PCGW/Wikidata
fallbacks cover rejected cases.

## Acceptance

- [ ] A truncated franchise-root query never binds an item whose digit
  tokens differ from the original title's ("The Witcher 3 …" rejects
  "The Witcher: Enhanced Edition"; "Riven - The sequel to Myst" still
  binds "Riven"; "doom" never subset-binds "Doom Eternal").
- [ ] A non-empty matching numeral set corroborates a weak truncated
  query: "the witcher" + {3} binds "The Witcher 3: Wild Hunt";
  "cyberpunk" + {2077} binds "Cyberpunk 2077".
- [ ] `go test ./...`, `go vet`, `gofmt`, GOOS=windows/darwin builds
  pass.

## Outcome

Shipped: `digitTokens`/`digitTokensEqual`/`tokenSubset` in
`internal/covers/covers.go`; `searchAppIDOnce` takes the original title
alongside the query variant and applies the veto + corroboration rule.
Four new tests in `internal/covers/search_variants_test.go` (red
witnessed: Witcher 1's 20900.img bound before the fix —
`tmp/test-red-033.log`). Full suite exit 0 (30 packages), vet/gofmt
clean, windows/darwin builds OK. Commit: pinned after the fact.
