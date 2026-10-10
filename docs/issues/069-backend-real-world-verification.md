---
title: Real-world backend verification (Bottles E2E; wineserver stop cleanup)
description: Bottles backend verified end-to-end on the dev box (007 First Light launches and renders via bottles-cli) — drove two fixes: bottle names come from bottle.yml (case-sensitive), and stops now kill the prefix's wineserver. Lutris/Heroic remain fixture-only (unconfigured/not installed).
status: done
---

# 069 — real-world backend verification

## What and why

The launchercli backends were fixture-tested only. Running the real
thing on the dev box found two real bugs the fixtures could not.

## Acceptance

- [x] Bottles E2E: 007 First Light (pinned to the custom-path bottle
  `wine11`) launches via `bottles-cli run -b Wine11 -e <exe>` and
  renders (vkd3d-proton pipeline setup in the launch log) — with
  umu untouched by the prefix.
- [x] Fix: the bottle OwnerRef is bottle.yml's `Name:` — Bottles
  lowercases dirs ("wine11") but bottles-cli matches names
  case-sensitively ("Bottle wine11 not found" observed).
- [x] Fix: stops kill the prefix's wineserver. Confirmed live that
  group kills miss wine processes (wineserver re-sessions its
  clients): the Bottles stopper now runs the configured runner's
  `wineserver -k` (direct runner-dir lookup — plain wine builds are
  rejected by the proton runner scan), and the umu cancel path runs
  `KillWineserver` with the launch runner.
- [x] Lutris: binary present but unconfigured (no prefixes) — backend
  stays fixture-tested. Heroic: not installed — fixture-tested.
- [x] `go vet`, `go test ./...` green; darwin/windows cross-compile OK.

## Outcome

`umu.BottleRunner` (bottle.yml `Runner:`) and `umu.KillWineserver`
(GE-Proton `bin/` and UMU-Proton `files/bin/` layouts) added; the
stopper registry carries prefix+source. The per-launch log again
proved itself: the wrong-case failure was one log read. Commit:
a1a1122.
