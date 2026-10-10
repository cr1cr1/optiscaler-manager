---
title: GAMEID/STORE resolution for protonfixes (umu-database)
description: umu launches resolve GAMEID/STORE per game — per-game override → umu-<SteamAppID> when identified → umu-database lookup for Heroic titles (cached 30d, degrades) → umu-default — so protonfixes apply per-game fixes.
status: done
---

# 064 — GAMEID/STORE resolution for protonfixes

## What and why

Every umu launch went out as `GAMEID=umu-default` with an empty STORE,
so protonfixes never applied per-game fixes. umu's own source is
explicit: "GAMEID is strictly required and the client is responsible
for setting this" — the launcher must resolve it.

## Acceptance

- [x] Resolution chain (`Session.UmuGameIDFor`): per-game override
  (`gameid_overrides`, JSON-edited, the SteamGridDBKey precedent) →
  `umu-<SteamAppID>` for identified games (the umu-database ID rule —
  Valve's appid-keyed fixes and protonfixes both apply) → umu-database
  lookup for Heroic titles (Epic AppName / GOG product id as codename)
  → `umu-default`. STORE maps epic→egs, gog→gog, steam→steam,
  manual→"".
- [x] New `internal/umudb` client: `umu_api.php?store=X&codename=Y`,
  30-day on-disk cache for hits AND misses (`ErrNoMatch`), transport
  failures never cached — lookups degrade, never block a launch.
- [x] The router passes the resolved pair into the launch env; debug
  logs carry gameid/store. End-to-end cmd test via the fake umu-run
  capture.
- [x] `go vet`, `go test ./...` green; darwin/windows cross-compile OK.

## Outcome

API schema verified against the umu-database repo README and a live
probe (`("egs","Heather")` → `umu-1174180`, miss → `[]`). The hook
gained a sixth resolver (`gameIDFor`) — the parameter list is now
debt-worthy of a resolver struct if it grows again. Commit:
28583b4.
