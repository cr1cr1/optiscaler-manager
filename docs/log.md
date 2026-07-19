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
