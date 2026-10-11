---
title: DLSS updates target the OptiScaler directory
description: DLSS DLLs install beside dlssg_to_fsr3_amd_is_better.dll (OptiScaler/ subdir for the dlssnr fork), never blindly into the injection dir.
---

# 081 — DLSS updates target the OptiScaler directory

## What and why

User report: with OptiScaler installed, a DLSS upgrade/install must
place the three NVIDIA runtime DLLs in the same directory that holds
`dlssg_to_fsr3_amd_is_better.dll`. For standard OptiScaler that is the
game bin dir (== the resolved injection dir, no change), but for the
dlssnr fork it is the `OptiScaler/` subdir — while `app.UpdateDLSS`
(and restore/snapshots) always wrote to the exe-heuristic injection
dir, creating a duplicate set in the wrong place that the game never
loads.

Fix: a shared DLSS-dir resolver — the directory containing
`dlssg_to_fsr3_amd_is_better.dll` when OptiScaler is installed (backup
stores skipped), else the existing injection-dir fallback. The three
`app` DLSS ops resolve through it, and the scan pill plus the post-op
UI refresh probe `DLSSVersion`/`DLSSReady` from that same dir, so the
displayed version is the version of the set the updater manages.

## Acceptance

- [x] `app.UpdateDLSS` on an NR-layout game (dlssg_to_fsr3 in `OptiScaler/`) replaces the three DLLs beside it and backs up there; nothing lands in the game root.
- [x] Scan entries report `DLSSReady` + `DLSSVersion` from the OptiScaler dir; games without OptiScaler keep today's injection-dir behavior.
- [x] `go test ./...`, `go vet`, gofmt clean.

## Outcome

Shipped: `app.ResolveDLSSDir` + ops/probe/scan wiring, red-first on
both agreed seams (writer + probe). Full suite, vet, gofmt clean.
Docs: scope v0.14 bullet, log entry.
Commit: 2b1d898.
