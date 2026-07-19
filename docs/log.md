# Log

Append-only milestone and task log. Newest at the bottom.

## 2026-07-19 — M0: hygiene / vendor / docs

- Added `docs_okf_test.go` (`TestDocsOKFFrontmatter`) as the failing test first
  (TDD red: `docs/ must exist and be readable`).
- Created OKF-compliant docs scaffold: `index.md`, `log.md`, `scope.md`,
  `architecture.md`, `safety.md`, `plan.md`.
- Dropped the dead `--config` flag from `RootFlags` (declared, never consumed;
  re-add when a reader exists).
- `go mod vendor` for the existing dependency set; `vendor/` committed so CI's
  `GOFLAGS=-mod=vendor` stops failing on clean checkouts.
- Plan deviation recorded: the plan's M0 said "add sevenzip, go-vdf, shirei to
  go.mod first". `go mod vendor` only vendors packages that are imported, and
  `go mod tidy` drops unimported modules — so dependencies are added (and
  vendored) in the milestone that first imports them: go-vdf in M2a,
  bodgit/sevenzip in M2d, shirei in M6. Each such milestone commits its own
  vendor update.
- README filled with project summary, status, and dev commands.

## 2026-07-19 — M2b: classify

- `internal/classify`: `Dir()` walks a game tree, matches DLL base names
  (case-insensitive) against the DLSS/DLSS-FG/FSR/XeSS table, dedups by
  (Kind, DLL), returns sorted components. Non-`.dll` files and `.git` skipped.
- TDD: `TestClassifyDetectsKnownComponentDLLs` red first, then green.
- Note: case-variant duplicates (e.g. `NVNGX_DLSS.DLL` vs `nvngx_dlss.dll`)
  are kept as separate entries ("as found" semantics).

## 2026-07-19 — M2c: gh client

- `internal/gh`: `Client.Resolve(ctx, requested)` →
  `(domain.ResolvedAsset, fromCache bool, err)` against the GitHub releases
  API; asset selected by glob `Optiscaler_*.7z` (never exact name);
  "latest" skips prereleases; missing tags fail loud.
- `Client.Download` streams to temp + rename, computing SHA-256 en route.
- Rate-limit handling: any API attempt starts a 15-min persisted cooldown;
  inside cooldown, resolution serves `releases.json` cache with
  `fromCache=true`; without cache → `ErrRateLimited`. No retry machinery.
- Tests: glob match, cooldown/cache, requested-vs-resolved separation, and
  a download SHA-256 test — all against httptest, zero real network.
- Deviation: `Release.Prerelease` added (required for "latest" semantics);
  client keeps an asset-name→URL index since `domain.ResolvedAsset` carries
  no URL.

## 2026-07-19 — M2a: discovery

- `internal/discovery`: `ParseLibraryFolders` (modern nested + legacy flat
  VDF forms), `ParseAppmanifest`, `ScanSteam` (root + all libraries, broken
  manifests and missing install dirs skipped, error only when no library is
  readable), `SteamRoots` (native/Flatpak/Snap candidates, symlink-deduped),
  and `ResolveInstallDir` (UE5 Phoenix/Binaries/Win64 rule first, else exe
  scoring: +15 name, +5 Binaries/Win64, +10 >5MiB, +25 upscaler adjacency
  via `internal/classify`; crash/redist/setup/installer/launcher/unins exes
  excluded; lexicographic tiebreak).
- Added `github.com/lewisgibson/go-vdf` (vendored). Its `Decode(&node)` raw
  tree API is used; case-insensitive key lookup covers both VDF casings.
- TDD: the test suite (salvaged from a blocked sub-agent run, reviewed) was
  in place first; implementation written against it. A rogue sub-agent
  dep-upgrade commit (testify + upgraded kong/x/sys) was reverted before
  this milestone was redone by the lead.

## 2026-07-19 — M1: domain types + external manifest store

- TDD: wrote `TestManifestJSONRoundTrip` and `TestStoreSaveLoadListManifests`
  first; red confirmed (`no non-test Go files`), then implemented to green.
- `internal/domain` (`game.go`, `release.go`, `manifest.go`): `Game`;
  `Kind`/`Component` (DLSS, DLSS-FG, FSR, XeSS); `ResolvedAsset`;
  `Manifest` with `Status` state machine (`in_progress`/`committed`/`failed`/
  `rolled_back`), `OpEntry`, `OverwrittenEntry`, `CreatedEntry`,
  `SchemaVersion = 1`; JSON tags are snake_case per task spec (safety.md's
  sketch used camelCase; structure follows safety.md exactly, plus the
  spec'd `ops[]`).
- `ManifestID`: first 16 hex chars of SHA-256 of the canonical install dir.
- `internal/store` (`store.go`): concrete on-disk store; manifests at
  `<root>/manifests/<id>.json` (atomic temp-write + rename, 0600), backups
  under `<root>/backups/<id>/`; `Save` stamps `UpdatedAt` (UTC); `List`
  sorted by ID, empty store is not an error; `DefaultRoot` = XDG data home
  (`$XDG_DATA_HOME/optiscaler-manager`, fallback `~/.local/share/...`).
- Stdlib only; no new dependencies, vendor/ untouched. Verified with
  `go test ./...` and `go vet ./...`, also under `GOFLAGS=-mod=vendor`.
