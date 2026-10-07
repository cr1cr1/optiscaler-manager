---
title: Install filters markdown, scripts, and empty dirs
description: Installing an OptiScaler bundle no longer copies markdown documentation or the distribution's own install/remove scripts into the game directory, and never creates empty directories.
---

# 026 — Install filters markdown, scripts, and empty dirs

## What and why

Installing a bundle used to copy *every* archive member into the game dir
(issue 010: the archive listing is the install set). Real bundles ship
launcher-side clutter — `README.md`, `CHANGELOG`, `setup_windows.bat`,
`Remove OptiScaler.bat` — that a game directory does not need. The plan
now filters markdown (`.md`, `.markdown`) and install/remove scripts
(`.bat`, `.cmd`, `.ps1`, `.sh`), case-insensitively. Empty directories
fall out for free: directory members were already skipped, and parent
dirs are created lazily per copied file, so a directory holding only
filtered files (e.g. a docs-only `docs/`) is never created.

The filter is plan-level (`buildPlan`), so it also applies to the
manifest file set: filtered members are never tracked, hence never
uninstalled or fork-switch-relocated either.

## Acceptance

- [x] `buildPlan` drops markdown and script members (case-insensitive)
  and keeps everything else verbatim, nested paths included.
- [x] End-to-end install of a cluttered bundle copies only the payload;
  no markdown/scripts in the game dir, none tracked in the manifest, and
  no empty `docs/` dir created.
- [x] The DLSSNR-shaped fixture carries realistic clutter so every
  fork-layout test also guards the filter.
- [x] Docs updated: `docs/scope.md`, `docs/architecture.md`, `docs/log.md`.

## Outcome

Done. One filter table (`filteredExts`) in `internal/installer/install.go`
consulted by `buildPlan`; no new required files, no config. The
foreign-modified relocation test switched its probe file from `README.md`
(no longer installed) to the tracked `OptiScaler/libxess.dll`. Full
`go test ./...` green, `go vet`/`gofmt` clean. Commit: see git log.
