---
title: Selectable OptiScaler distribution forks in Settings (GUI + TUI)
description: Settings gains a fork list (slug + asset glob) with add/delete and an active selection; downloads, caches, and manifests become fork-aware; the first bundled alternative is the DLSSNR-PreSR-Multipass fork.
issue: 5
status: done
---

# 5 — Selectable OptiScaler distribution forks in Settings (GUI + TUI)

## What and why

The OptiScaler source was hardcoded to upstream
(`optiscaler/OptiScaler`, assets `Optiscaler_*.7z`). Users want to
install from forks — the first being
`jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass`, which ships
`OptiScaler-NR-*.zip` assets — and to manage their own fork list. The
choice is global (one active fork drives every install/switch), each
install records the fork it came from, and forks are addable/removable
from both frontends.

Design (brainstormed and approved 2026-10-06): each fork is a
`settings.Fork{Slug, AssetPattern}`; `gh.NewFork` talks to an arbitrary
repo with a `path.Match` asset glob; the cache namespaces per fork
(`cache/optiscaler/<fork-key>/`) so same-named tags from different
distributions can never collide; `internal/archive` dispatches `.7z` vs
`.zip` on extension; manifests record the fork slug; the session swaps
its GitHub client through a `NewGH` factory when the active fork changes.

## Acceptance

- [x] Settings schema: `Forks` + `ActiveFork`, built-ins seeded, upstream
      undeletable, legacy files normalize; add/delete validated.
- [x] `gh` resolves/downloads from an arbitrary fork with its asset glob.
- [x] `archive` extracts `.zip` with the same hostile-input defenses.
- [x] Per-fork cache namespaces; legacy cache migrates best-effort.
- [x] Manifest + library rows carry the fork slug.
- [x] Session: `SetActiveFork`/`AddFork`/`RemoveFork` persist, swap the
      client, clear fork-scoped memos.
- [x] CLI one-shots install from the active fork.
- [x] GUI settings modal: fork list, select-active, add, delete.
- [x] TUI settings screen: fork list, select-active, add, delete.
- [x] `go test ./...` green; docs updated.

## Outcome

- `settings.Fork{Slug, AssetPattern}` with built-ins
  (`optiscaler/OptiScaler` + `Optiscaler_*.7z`,
  `jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass` + `OptiScaler-NR-*.zip`),
  `Forks`/`ActiveFork` on `Settings`, `Load` normalization (legacy files
  seed the built-ins, a dropped upstream is restored, a dangling
  ActiveFork resets), `AddFork`/`RemoveFork` validation, and
  `ForkKey`/`BundleCacheDir` for the per-fork cache namespaces
  (`cache/optiscaler/<owner>__<repo>/`; the legacy flat layout migrates
  via `app.MigrateLegacyBundleCache`, best-effort, collision-safe).
- `gh.NewFork`/`NewForkWithBaseURL` take slug + `path.Match` glob
  (empties = upstream, so the `OM_GH_BASE_URL` seam and every existing
  constructor call are unchanged); `selectAsset` matches the glob.
- `internal/archive` dispatches `.7z`/`.zip` on extension through one
  entry pipeline; same sanitize/dup/size defenses both formats.
- `domain.Manifest.Fork` recorded by `installer.Install`;
  `app.InstallOpts.ForkSlug` picks the cache namespace and the recorded
  slug (empty = upstream); `LibraryEntry`/`GameRow` carry it, with
  `GameRow.ForkLabel()` rendering the repo segment on the GUI badge and
  the TUI detail view for non-upstream installs.
- Session: `Deps.NewGH` factory, `ghClient()` accessor (fork switches are
  race-free), `SetActiveFork`/`AddFork`/`RemoveFork` persisting through
  one `persistSettings` helper; a fork switch swaps the client and clears
  the startup-latest and resolved-default memos.
- CLI: `newDeps` builds the active-fork client and the factory, migrates
  the legacy cache; `install` passes the active slug. GUI: settings modal
  "OptiScaler Sources" section (Use/Remove per row, slug+glob add
  inputs). TUI: settings screen "OptiScaler sources" list (tab toggles
  list focus, enter activates, a/d add/remove with inline confirm,
  upstream removal refused).
- Tests (TDD, red witnessed at each step): settings fork CRUD +
  normalization, gh fork resolve/download + no-match error, zip
  extract/hash/traversal/unknown-format, per-fork CachedVersions,
  installer + app fork manifests and cache namespacing, session swap /
  memo invalidation / persistence / row propagation, legacy cache
  migration (incl. collision), GUI section render + keyboard select +
  add/remove, TUI list/select/add/remove.
- ponytail-review folded in: duplicate toast helper deleted, the three
  fork mutators share `persistSettings`, `ValidateFork` inlined into its
  only caller.
- `go test ./...` green, `go vet` clean, `gofmt` clean. README,
  docs/scope.md (distribution forks section), docs/architecture.md
  (gh/archive/settings package lines) updated (b157d08).
- Prerequisite: issue 4 (shirei v0.8.0 migration) landed first; the
  workspace's half-done dep bump blocked the GUI half of this work.

