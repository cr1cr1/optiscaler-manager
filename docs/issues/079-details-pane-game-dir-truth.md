---
title: Details pane reconciles install truth from the game directory
description: Manager state is never the truth — the details pane displays install state verified live against the actual game directory.
---

# 079 — Details pane reconciles install truth from the game directory

## What and why

User directive after the issue-076/077 uninstall flow deleted a file the
game's current setup still needed: **the optiscaler-manager state is
never the truth; it must always be reconciled from the actual game
directory, on details pane display.**

Today the details pane renders the cached row status: `probeInstallState`
only fills an EMPTY status, so a `committed` row is displayed as
installed even when the directory was since modified by hand (files
changed, ripped out, hooks swapped) — the manager's records are shown
as fact while silently disagreeing with reality.

Fix: `app.ReconcileInstall` verifies the selected game against its
directory on every details-pane open (both frontends already route
through `Session.RefreshInstallState`): manifest tracked files are
re-hashed (verified/changed/missing), the injection hook and its
disabled twin are probed, and the result rides the session snapshot as
`State.DetailTruth` (async, `EvDetailTruth` poke). The GUI detail panel
and the TUI detail view render a "Game dir:" line from that truth — the
directory's word, displayed alongside (and overriding in meaning) the
manifest-derived status. Post-op, the selected game re-reconciles.

## Acceptance

- [x] `ReconcileInstall` reports managed verified/changed/missing counts,
      external hook detection, and clean absence from directory facts
      alone (unit-tested with a hand-built manifest).
- [x] Selecting a game reconciles: a foreign-modified file shows as
      "changed" on the details pane while the manifest still claims
      committed (the Witcher 3 scenario).
- [x] GUI detail panel and TUI detail view both render the truth line.
- [x] `go test ./...`, `go vet`, gofmt clean.

## Outcome

Shipped as designed. `app.ReconcileInstall` (read-only directory
verdict) + `State.DetailTruth`/`EvDetailTruth` session plumbing +
"Game dir:" lines in both frontends via shared `ui.TruthLine`. Tests:
`TestReconcileInstall_ManagedCounts`/`_Unmanaged` (app),
`TestSelectReconcilesDetailTruth` + `TestTruthLineShapes` (session),
`TestDetailViewRendersGameDirTruth` (TUI string),
`TestDetailPanelRendersGameDirTruth` (GUI rect). Full suite 36/36
green on rerun (one `TestClearCoverOverrideRestoresChain` TempDir-race
flake, pre-existing), vet + gofmt clean.

Commit: THIS_COMMIT.
