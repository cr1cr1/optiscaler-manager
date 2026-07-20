---
type: reference
---

# Implementation plan (v0.1)

Sequenced, TDD-gated milestones. One conventional commit per milestone, only
after `go test ./...` is green. Full suite always; `go vet ./...` each
milestone; golangci-lint from M3 onward.

## Waves

| Wave | Milestones | Mode |
|------|-----------|------|
| 1 | M0 hygiene/vendor/docs | sequential |
| 2 | M1 domain+store | sequential |
| 3 | M2a discovery ∥ M2b classify ∥ M2c gh ∥ M2d archive spike | parallel |
| 4 | M3 installer core | sequential (needs M2c, M2d) |
| 5 | M4 profile+EAC | sequential |
| 6 | M5 CLI | sequential |
| 7 | M6 GUI | sequential |
| 8 | M7 polish | sequential |

Critical path: M0 → M1 → M2d → M3 → M4 → M5 → M6 → M7.

## Milestones

- **M0 — hygiene/vendor/docs.** Test first: `TestDocsOKFFrontmatter`. Vendor
  existing deps; drop dead `--config`; docs scaffold; README.
  Commit: `chore: vendor deps, scaffold OKF docs, drop dead --config flag`.
- **M1 — domain+store.** Tests: `TestManifestJSONRoundTrip`,
  `TestStoreSaveLoadListManifests`. Files: `internal/domain/`, `internal/store/`.
  Commit: `feat(domain): manifest schema and external manifest store`.
- **M2a — discovery.** Tests: `TestParseLibraryFolders`, `TestParseAppmanifest`,
  `TestResolveInstallDirPrefersUE5Win64`, `TestExeScoringSkipsCrashRedistSetup`.
  Adds go-vdf (+vendor). Commit: `feat(discovery): steam library scan and
  install-dir resolution`.
- **M2b — classify.** Test: `TestClassifyDetectsKnownComponentDLLs`
  (table-driven). Commit: `feat(classify): detect DLSS/FSR/XeSS by DLL`.
- **M2c — gh client.** Tests: `TestResolveReleaseMatchesAssetGlob`,
  `TestRateLimitCooldownServesCachedReleases`,
  `TestRequestedVsResolvedRecordedSeparately`. Commit: `feat(gh): release
  resolution with glob asset match and cooldown cache`.
- **M2d — archive SPIKE GATE.** Test: `TestSevenzipExtractsRealOptiScaler094Archive`
  against a real 0.9.4 asset (blocks M3). On BCJ2 failure: shell-out backend +
  `TestSevenZipCommandAvailableReportsActionableError`. Adds bodgit/sevenzip
  (+vendor) if spike passes. Commit: `feat(archive): 7z extraction backend`.
- **M3 — installer core.** Exactly five fault-injection tests (see
  `safety.md`). Commit: `feat(installer): transactional install with manifest,
  backups, rollback`.
- **M4 — profile+EAC.** Tests: `TestDefaultINISafeDefaults`,
  `TestEACProtectedDetectsStartProtectedGame`. Commit: `feat(profile): curated
  OptiScaler.ini defaults and EAC check`.
- **M5 — CLI.** Tests: `TestScanCommandListsGames`,
  `TestInstallCommandRunsTransaction`, `TestStartupRecoveryFlagsInterruptedManifests`.
  Commit: `feat(cmd): headless scan/install/uninstall with startup recovery`.
- **M6 — GUI GATE.** Tests: `TestActionListSortsActionableFirst`,
  `TestFilterNarrowsList`, `TestEACModalShownBeforeInstall`,
  `TestRenderToPNGSmoke`; run with `-race`. Adds shirei v0.5.2 (+vendor).
  Commit: `feat(gui): action-list window with per-game install dashboard`.
- **M7 — polish.** golangci-lint clean; `goreleaser release --snapshot` builds
  (vendored); README usage; log final entry. Commit: `docs: readme, milestone
  log, release verification`.

## Spike gates

1. **M2d real-archive extraction** — gates M3. BCJ2 filter support in
   bodgit/sevenzip is unverified; the fallback (shell out to `7z`) is
   pre-planned.
2. **M6 GUI smoke** — gates M7.

## v0.4 milestone (complete, 2026-07-20)

Settings UI, games cache, GUI polish, TUI overhaul. All gates green
(`go test ./...`, `go vet ./...`); scope recorded in `docs/scope.md`,
task detail in `docs/log.md`.

- **W1 (T1)**: games.json library cache, `Session.Start` cache-first boot
  with manifest status reconcile, `RemoveDirectory`, `SetSort`.
- **W2a (T2)**: GUI polish (theme tokens, hover states, gradient
  placeholders, detail side panel, sort menu, icon view switch, empty
  states, arrow-key nav, raised toasts) + settings scan-directory list and
  launch-template editing + cache-first GUI boot.
- **W2b (T3)**: TUI overhaul (number-key screens, styled columns, detail
  screen, live filter, settings directory management, confirm modal,
  spinner, toasts) on bubbles v1.0.0 + direct lipgloss.
- **Fix**: `settings.Save` is a no-op on an empty root (sessions without a
  state dir must not fail or spam callers).

## Risks

1. BCJ2/sevenzip failure → early spike, fallback ready.
2. shirei alpha churn → quarantine under `internal/gui`, pinned, deliberate upgrades.
3. Proton path variance → paths as data, table-driven fixtures, fail loud.
4. GitHub rate limits → cooldown + cache + explicit substitution prompt.
5. Frame-goroutine races → `WithFrameLock`/`RequestNextFrame`, channels, `-race` in M6.
