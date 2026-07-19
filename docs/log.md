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
