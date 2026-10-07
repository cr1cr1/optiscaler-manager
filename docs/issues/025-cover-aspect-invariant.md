---
title: Enforce a 2:3 aspect invariant on cached covers (center-crop), ending stretched posters
description: Games whose only art is a landscape Steam hero banner (Aphelion, Resonance A Plague Tale Legacy) render stretched because the aspect-blind ImageFill renderer fills the portrait card; the covers package now normalizes every cached image to the 2:3 card aspect via center-crop — on write and on read (legacy scrub) — so no source can ever inject stretchable art again.
---

# 025 — 2:3 aspect invariant for cached covers

## What and why

User report: some posters render stretched vertically (Aphelion,
Resonance: A Plague Tale Legacy) — with the explicit instruction to fix
the overarching issue, not individual titles.

Root cause: those games have no portrait art on Steam's CDN
(`library_600x900.jpg` 404s) so the chain lands on the landscape
`library_hero.jpg` (1920×620, ≈3.1:1), which is cached as the cover —
and the GUI's `ImageFill` (v0.14 vendor patch) stretches whatever it is
given to fill the ~2:3 card. The patch's premise ("a gap is worse than
minor distortion") holds for true 2:3 posters and breaks for anything
else. **The pipeline has no aspect invariant:** any non-portrait image
any source produces — hero banners today, anything tomorrow — gets
stretched by the aspect-blind renderer.

Fix (user decision): **enforce the invariant at the cache boundary** in
the covers package, by **center-crop** (the user chose crop over
letterbox; Steam hero art is center-composed, and crop is what
Lutris/Playnite do):

- Every image written to the cover cache is normalized to the 2:3
  portrait aspect: already-2:3 art (Steam/PCGW/SGDB 600×900 posters)
  passes through byte-identical; anything else is center-cropped and
  re-encoded (JPEG stays JPEG, PNG stays PNG, other decodable formats —
  e.g. SGDB webp — become PNG).
- The cached-hit path re-checks aspect on read and scrubs in place, so
  legacy landscape images cached before this fix self-heal on the next
  scan — one owner, idempotent, no cache-version bump, no vendor-patch
  changes, all frontends fixed at once.
- Undecodable content (error pages saved as art, truncated downloads)
  is left alone — `ImageFill` already tolerates unloadable files.

## Acceptance

- [x] A landscape hero fallback (e.g. 300×90) is cached as a 2:3
  portrait image; the center band survives, the side bands are cropped.
- [x] A 2:3 source image (any already-portrait art) is cached
  byte-identical (no re-encode quality loss).
- [x] A legacy landscape image already in the cache is normalized in
  place when read (self-healing scrub).
- [x] Non-image bytes in a cached file are a graceful no-op (no panic,
  no rewrite).
- [x] Full `go test ./...` green, `go vet`/`gofmt` clean,
  `GOOS=windows`/`darwin go build ./...` OK.
- [x] Docs: this issue, `docs/log.md`, `docs/architecture.md`,
  `docs/scope.md`, `README.md`.

## Outcome

Implemented as specced: `normalizeCover` (covers package) enforces the
2:3 invariant on every fetch and every cached hit; ~2:3 art passes
through byte-identical (1% tolerance), anything else is center-cropped
and re-encoded (JPEG→JPEG q90, other formats→PNG; webp decode via
`golang.org/x/image/webp`, which stays `// indirect` in go.mod —
`go mod tidy && go mod vendor` would strip the vendored shirei patches
and was deliberately not run). ATDD red witnessed on the two landscape
tests before implementation. Commit: `eb54a18`.
Verification: `go test ./...` (30 packages) exit 0, `go vet`/`gofmt`
clean, `GOOS=windows`/`darwin go build ./...` OK.
