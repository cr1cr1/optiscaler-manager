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

## 2026-07-19 — M2d: archive backend + SPIKE GATE

- `internal/archive`: `List`, `ExtractTo` (staging extraction with
  hostile-input defenses), `SanitizeName`, `HashEntry`, `EntryNames`.
  Defenses: absolute/UNC/drive-letter/traversal names rejected, backslash
  trickery normalized, case-folded duplicates rejected, non-regular entries
  (symlinks/hardlinks) rejected, per-file (1 GiB), total (4 GiB), and
  entry-count (100k) caps.
- SPIKE GATE RESULT (real `Optiscaler_0.9.4-final.20260718._MM.7z`, 55 MB,
  23 entries): **bodgit/sevenzip decodes it fully, including the BCJ2
  filter** — OptiScaler.dll decompressed to 25,379,632 bytes with stable
  SHA-256, full extraction into staging clean. Backend = pure-Go sevenzip;
  the shell-out-7z fallback is NOT needed.
- Ground-truth correction: the 0.9.4 bundle ships `fakenvapi.dll` (not
  `nvapi64.dll` as the older C# client expected). Bundle layout: files at
  archive root + `D3D12_Optiscaler/` + `Licenses/`.
- Ponytail deviation: no `Backend` interface — a single implementation
  exists; introduce the interface only if a second backend ever appears.
- `TestSevenzipExtractsRealOptiScaler094Archive` stays env-gated
  (`OM_TEST_ARCHIVE`); skips in normal runs.

## 2026-07-19 — M3: installer core

- `internal/installer`: `Install` (plan → stage → validate → manifest-first →
  per-file backup/copy with progressive manifest saves → committed),
  `Rollback` (verified restore + conditional delete, idempotent),
  `Uninstall` (SHA-matched delete/restore, `RefusedError` on foreign bytes,
  manifest+backup cleanup on success), `buildPlan` plan-time hostile-input
  gate, `copyFileFn` fault seam (white-box tests only).
- Crash-point design: created/overwritten entries are pre-registered with
  empty hashes so rollback can distinguish "never touched", "partial write"
  and "completed"; leftover in_progress/failed manifests are auto-rolled-back
  before a fresh install; committed manifests refuse re-install.
- store extended: `StagingDir`, `Delete` (idempotent).
- Five settled fault tests plus a byte-for-byte round-trip test, all green.
  Fixture: `internal/installer/testdata/bundle.7z` (2 KB, minted once with
  system 7z, mirrors the real bundle layout).
- errcheck cleanup in store.go + gh/client.go (M1/M2c vintage); golangci-lint
  gate now active and clean.
- Note: two stray commits from the blocked M2a sub-agent's detached loop
  (`f827cd4` action version bumps, `7abbb6c` golangci-lint pin) were reviewed
  and kept as benign CI housekeeping; its earlier dep-upgrade damage had
  already been reverted.

## 2026-07-19 — M4: profile + EAC

- `internal/profile`: `WriteDefaultINI` — curated minimal safe-defaults ini
  (upscalers + FG all `auto`), deterministic output.
- Install integration: after the file plan completes, the bundle's
  OptiScaler.ini is atomically replaced with the curated ini and the
  manifest entry hash is refreshed (`profile` op).
- `installer.EACProtected`: detects `start_protected_game.exe` in the game
  root; the GUI/CLI must warn before installing into such games.
- Tests: `TestDefaultINISafeDefaults`, `TestInstallWritesCuratedINI`,
  `TestEACProtectedDetectsStartProtectedGame`. Lint clean.

## 2026-07-19 — M5: CLI commands + startup recovery

- `cmd/scan.go` (`scan`): Steam roots auto-detect or `--steam-root` /
  `OM_STEAM_ROOT`; prints name, appid, upscaler kinds (via classify),
  install dir, `[EAC]` marker.
- `cmd/install.go` (`install <path>`): ResolveInstallDir → EAC refusal
  (`--force` overrides) → gh.Resolve latest (rate-limit cache refused unless
  `--allow-cached`) → Download with SHA-256 → installer transaction →
  committed manifest. `uninstall <path>` and `rollback <path>` reverse via
  the manifest (installer.ManifestIDFor helper added).
- `cmd/deps.go`: `Deps` (Out/ErrOut/Store/CacheDir/GH/Version) injected via
  kong bindings; `newDeps` honors `OM_DATA_DIR` and `OM_GH_BASE_URL`;
  `checkInterrupted` startup-recovery hook warns on in_progress/failed
  manifests before dispatch.
- `gh.NewWithBaseURL` exported for the env override and tests.
- `version` now prints through `Deps.Out` (uniform `kctx.Run(deps)`
  dispatch).
- Tests: `TestScanCommandListsGames`, `TestInstallCommandRunsTransaction`
  (end-to-end against httptest GitHub + fixture bundle, incl. uninstall),
  `TestStartupRecoveryFlagsInterruptedManifests`. Lint clean.

## 2026-07-19 — M6: GUI (shirei Action List)

- `internal/app` extracted as the shared orchestration both frontends
  consume (second-consumer rule): `ScanLibrary` (scan + classify + EAC +
  status + mtime + InjectionDir enrichment), `Install` (resolve → download →
  transaction, `InstallOpts{AllowCached, EACOverride}`, typed
  `ErrEACProtected`), `Uninstall`, `Rollback`, `ManifestIDFor`. cmd commands
  refactored onto it; all M5 tests stayed green.
- `internal/gui` (the only shirei importer): pure view model
  (`sortRows` actionable-first then recency, `filterRows` substring/appid,
  `decideInstall` EAC gating) + shirei views: filter TextInput, VirtualListView
  Action List, per-game dashboard Modal (install/uninstall/rollback/open-ini),
  EAC confirmation Modal, `--audit-grid` raw Table. Background work mutates
  state under `WithFrameLock` + `RequestNextFrame`; domain packages never
  import shirei.
- `cmd/gui.go`: `Gui GUICmd \`cmd:"" default:"withargs"\`` — bare
  `optiscaler-manager` launches the GUI.
- Deps: go.hasen.dev/shirei **v0.5.2 pinned** (vendored). `f32` is an
  unexported alias upstream; `float32` used directly.
- Lint: ST1001 (dot imports) excluded for `internal/gui/` only — shirei's
  documented idiom; exclusion lives at `linters.exclusions` (v2 config).
- Tests: `TestActionListSortsActionableFirst`, `TestFilterNarrowsList`,
  `TestEACModalShownBeforeInstall`, `TestRenderToPNGSmoke` (headless 5.5KB
  PNG). Full suite + `-race` + golangci-lint all green.
- Not covered by automated tests: the real window launch (needs a display;
  `go run .` is banned by repo rules). First manual launch is a user
  checkpoint.

## 2026-07-19 — M7: polish / release verification

- README rewritten: real usage (all subcommands), EAC/rate-limit refusal
  semantics, state-root location, env-var table, dev commands.
- `goreleaser release --snapshot --clean` — **succeeded**: linux + windows
  amd64 archives build vendored with CGO off (one-time release-config gate;
  the no-build rule governs the dev loop, not release tooling).
- arm64 and darwin are commented out in the skaffold's `.goreleaser.yml`;
  enabling arm64 later is a one-line change (all deps are pure Go on
  linux/windows; darwin needs CGO).
- Final gates: `go test ./...`, `go vet ./...`, `golangci-lint run` — all
  green. v0.1 scope complete: M0–M7 done.

## 2026-07-19 — P1: covers (v0.2 GUI restyle starts)

- `internal/covers`: `Cover(ctx, appID, name)` — Steam CDN
  (`library_600x900.jpg` by appid) primary, Steam store search (name→appid)
  fallback, generated dark-tile PNG placeholder on total miss. On-disk cache
  keyed by sanitized appid (digits only — hostile input can't escape the
  cache dir); atomic downloads; 32 MiB cap.
- Chain decided with the user: Steam CDN primary, other services as
  fallback (Bottles/Heroic source survey pending; store-search is the
  zero-key fallback the C# client also uses).

## 2026-07-19 — P2: ui Session core (frontend-agnostic)

- `internal/ui`: the abstraction the TUI/CLI will share. `Session` holds
  all interactive state (`State{Rows, Query, Mode, Selected, Busy,
  StatusLine, Confirm, Toasts}`) and exposes async commands (`Scan`,
  `QuickInstall`, `Install`, `Uninstall`, `Rollback`, `SetQuery`,
  `ToggleView`, `Select`, `AnswerConfirm`, `OpenINI`) plus a buffered event
  stream (`Events()`) and `Snapshot()` for rendering.
- `GameRow` is display-ready: title, badges (`Badge{Label, Tone}` — tones
  mapped per frontend), status, actionable, EAC, cover path. `sortRows` /
  `filterRows` moved here from the v0.1 GUI view model (dedup in P3).
- Consent model: EAC and stale-cache installs PAUSE with a `Confirmation`
  and cannot proceed until `AnswerConfirm(true)` — the frontend renders the
  prompt, the core enforces the gate (settled safety rule).
- Supporting seams: `app.ErrStaleCache` sentinel, `covers.NewWithBase`.
- Tests (all against fakes, zero real network): scan populates rows, quick
  install toggles both directions, EAC blocks until consent, stale cache
  requires consent, toast lifecycle + expiry, open-ini opener call,
  sort/filter semantics. `-race` clean.

## 2026-07-19 — P3: GUI refactored onto ui.Session

- `internal/gui` is now a thin shirei binding: `model` holds only the latest
  `ui.State` snapshot + the filter text buffer; views render `ui.GameRow`
  verbatim and forward commands (`QuickInstall`, `Rollback`, `OpenINI`,
  `Select`, `AnswerConfirm`, `SetQuery`). The v0.1 view model (sort/filter/
  decide logic) was deleted — it lives in `internal/ui` now.
- Dashboard drives the session's QuickInstall toggle; the confirm modal
  renders `ui.Confirmation` (EAC/stale-cache) and answers it.
- `cmd/gui.go` builds the session (store, gh, covers, cache) and starts a
  scan on launch.
- Bug found by the binding test: `drain()` must snapshot unconditionally
  after consuming events (previously a caller-side read could leave the
  model stale forever).
- Tests: `TestGUIBindsSessionState`, `TestGUIFilterSyncsToSession`, PNG
  smoke (list + confirm-modal frames). Sort/filter/EAC unit tests live in
  `internal/ui` since P2.

## 2026-07-19 — P4: cover-card grid + view toggle

- `internal/gui/grid.go`: the C#-client-style card grid — cover image
  (170×255, `Image` scales down to fit), platform pill, `✦ OptiScaler`
  installed badge, EAC marker, title, tech badges, quick-install toggle
  (`quickLabel`: Install/Uninstall by status). Cards chunk into rows of N
  (`chunkRows`, N recomputed from live width each frame) inside
  `VirtualListView`.
- `GameRow.Platform` added ("Steam" — discovery is Steam-only for now).
- Grid is the default view; header button toggles grid↔list via
  `Session.ToggleView` (`ui.ViewMode`).
- Key shirei finding (documented in view.go): virtualized views must be
  DIRECT Viewport children — inside auto-sized wrapper columns
  (Grow/Expand/Clip all fail) they render nothing; only explicit FixSize
  wrappers work otherwise. Verified against upstream demos.
- Visual QA loop: headless `RenderToPNG` reviewed as an image — grid,
  badges, covers confirmed rendering.
- Tests: `TestChunkRows`, `TestQuickInstallButtonLabelByStatus`,
  `TestGridSmoke`, `TestGridToggleRendersListMode`, `TestToggleView` (ui).

## 2026-07-19 — P5: chrome (dark theme, sidebar, toolbar, toasts, status bar)

- `internal/gui/theme.go`: dark HSLA palette (bg app/panel/card, main/muted/
  warn text), `txt`/`muted` helpers, `badgePill` with per-tone colors
  (DLSS green, FSR red, XeSS blue, installed purple, platform gray).
- `internal/gui/chrome.go`: icon sidebar (logo, Games, About → version
  modal), toolbar (Scan Games → `Session.Scan`, search, view toggle, busy
  text), full-width status bar (status line + game count), toast overlay
  (Float/Z, bottom-right, warn styling).
- Layout semantics discovered (bites hard, now documented): shirei's
  `Expand` fills the parent's CROSS axis only; in a Row that's height, not
  width. Full-width inner columns inside a Row need `Grow(1)+Expand`;
  full-width bars inside a column need `Expand`. Bare Row containers
  shrink-wrap to content.
- Visual QA: iterated `RenderToPNG` frames against the reference screenshot
  until sidebar, grid, pills, toasts, and status bar all render correctly.

## 2026-07-19 — P6: docs + cover-chain validation

- Cover-chain research (user question — where does Bottles get covers?):
  **Bottles uses a private proxy** (`steamgrid.usebottles.com/api/search/{name}`)
  wrapping SteamGridDB with the key server-side — not a copyable keyless
  source. Ecosystem validation instead: **Lutris and Heroic both hit Steam
  CDN keyless** (`library_600x900.jpg` / `header.jpg` by appid) — our exact
  primary; SteamGridDB elsewhere always needs a user key (Heroic model,
  stays cut). Our chain (Steam CDN → store search → placeholder) is the
  proven pattern; no change.
- docs/scope.md: v0.2 section (grid, covers, chrome, ui.Session, deferred).
  docs/architecture.md: package map + data flow with Session. README: v0.2
  status and frontend-abstraction note.

## 2026-07-19 — P7: real-machine GUI hardening (niri/tiling WM)

- First real launch (user-authorized `go run .`, niri screenshots) exposed
  three defects invisible to headless smoke tests:
  1. **Non-games in the library**: Steam Linux Runtimes, Proton, Steamworks
     redistributables, SteamVR, Wallpaper Engine etc. appeared as installable
     games. Fixed with `discovery.isNonGame` (name-substring exclusion list
     + appid 228980), TDD `TestScanSteamSkipsNonGames` — the same exclusion
     classes the reference client ships.
  2. **Horizontal overflow in narrow tiles**: fixed-size cards and a
     fixed-width search bled past the window edge. Cards are now adaptive
     (`cardContentH(cardW)`, cols from live width, `Clip` on rows); search is
     `Grow(1)` with min/max bounds.
  3. **Card content clipped**: tech pills and the install button didn't fit
     the computed card height; height now budgets cover + all rows.
- Also: dark button theming via `widgets.ButtonAccent`
  (`ContrastingTextColor` picks light text automatically).
- Process discipline: `pkill -f "go run ."` matched the shell's own command
  line and killed it — relaunch via `setsid`, kill by window id
  (`niri msg action close-window --id`).

## 2026-07-19 — P8: bundle cache, settings, manual add (user requests)

- **Versioned bundle cache**: bundles now live at
  `$XDG_CACHE_HOME/optiscaler-manager/optiscaler/<version>/` and are reused
  (hash-checked) before any download; second installs of the same version
  do zero network IO for the bundle. `OM_CACHE_DIR` overrides; covers moved
  under the cache root as well. Installer no longer clobbers the
  artifact-level SHA-256 with the staged-DLL hash when one is supplied.
- **Settings** (`internal/settings`, persisted `settings.json`): default
  OptiScaler version (tag or `latest`) and manually added dirs. Session
  commands `SetDefaultVersion` / `ClearBundleCache`; `app.InstallOpts.
  Requested` plumbs the configured tag into Resolve and the manifest.
- **Manual add**: `internal/pickdir` shells the OS directory dialog
  (zenity → kdialog; `ErrUnavailable` otherwise), `app.ManualEntry` builds
  the row, session `AddDirectory` dedupes and persists (ExtraDirs survive
  rescans).
- **GUI**: Settings sidebar button + modal (version input, clear cache),
  "Add Game" toolbar button. Dark modal cards — upstream `widgets.Modal`
  hardcodes a white surface, so `internal/gui` ships a local `modal()`;
  `widgets.ButtonAccent`/`DefaultBackground` themed at init.
- Tests: cache hit/miss (zero-download reuse), settings round-trip,
  SetDefaultVersion persistence + manifest tag, ClearBundleCache, manual
  add/dedupe/rescan, picker-unavailable path, settings-modal smoke.

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

## 2026-07-20 — W3-T1: pever — PE version parser, marketing maps, OptiScaler version chain

- TDD: red-first tests across `pe_test.go`, `versionmaps_test.go`,
  `optiscaler_test.go`, `real_archive_test.go` before the implementation
  (commit dae1360).
- `internal/pever` (new, stdlib only, no cgo): hand-rolled PE32/PE32+
  version-resource parser (`pe.go`). Every input is treated as hostile: all
  reads bounds-checked, malformed input yields the sentinels `ErrNotPE` /
  `ErrNoVersionInfo`, never panics. `FileVersion` normalizes the raw
  version: the ProductVersion string wins unless it is the placeholder
  1.0 / 1.0.x (then the fixed FILEVERSION quad), commas become dots, a
  leading "FSR " prefix is stripped, surrounding whitespace and one leading
  "v" trimmed.
- `versionmaps.go`: vendored lookup tables mapping raw DLL versions to
  vendor marketing names (DLSS/FSR/XeSS — e.g. 3.7.10.0 → "DLSS 3.7.10");
  `MarketingName(kind, raw)` is the public seam.
- `optiscaler.go`: `OptiScalerVersion(dir)` resolves the installed
  OptiScaler version via an evidence chain — `manifest.json` version beats
  the `OptiScaler.log` banner, which beats ini presence (install proven,
  version unknown).
- Verified: `go test ./...`, `go vet ./...`. No changes outside
  `internal/pever`.

## 2026-07-20 — W3-T2: per-store per-OS game launch (internal/launch)

- TDD: wrote `launch_test.go` first (red: `no non-test Go files`), then
  implemented to green. Test ids: `TestCommandTable_AllStoresAllOS`
  (steam/gog/epic/manual × linux/windows/darwin, exact argv),
  `TestCommandFallbackChain` (steam → flatpak → xdg-open; rundll32 on
  windows), `TestTemplateSplitAndPlaceholders`, `TestTemplateNoShellExpansion`,
  `TestLaunchCapturesArgvViaInjectedRunner`, `TestLaunchURLTimeout`, plus
  `TestCommandErrors` for the sentinel wrap paths.
- `Command(t)` is the pure, table-testable core; `Launch` builds and fires
  via an injected `Runner`. Steam linux: `steam steam://rungameid/<id>`
  (native `-applaunch` when args), flatpak and xdg-open fallbacks use the
  `steam://run/<id>//<args>/` URL form for args; steam windows falls back to
  `rundll32 url.dll,FileProtocolHandler`; darwin uses `open`. GOG is direct
  exe on all OS (DRM-free), Galaxy `/command=runGame` form on windows when
  only AppID+Dir known. Epic launches via
  `com.epicgames.launcher://apps/<AppName>?action=launch&silent=true`
  (AppName from the .item manifest), exe fallback when AppName empty.
  Manual expands `{exe}`/`{dir}`/`{appid}`/`{args}` in the user template
  (default `"{exe}" {args}`), split on double-quote grouping only — no
  shell, no metachar expansion; umu-run appears only if the user wrote it.
- Build-tagged spawners: `spawn_linux.go` (`Setsid`), `spawn_windows.go`
  (`CREATE_NEW_PROCESS_GROUP|DETACHED_PROCESS`, consts local),
  `spawn_darwin.go` (plain). Games: `Start` + `Process.Release`, never
  Wait; Dir set; stdio nil. URL openers (xdg-open/open/rundll32): 10s
  context cap and waited so failures surface. Logs say "launch requested",
  never "launched" — spawn success ≠ game running.
- Sentinels `ErrNoStore`/`ErrMissingExe`/`ErrMissingAppID`/
  `ErrMissingAppName`, wrapped with `%w`. Stdlib + zerolog only; go.mod and
  vendor/ untouched. Verified: `go test ./...`, `go vet ./...`,
  `golangci-lint run` (0 issues), `GOOS=windows go vet ./internal/launch/`,
  `GOOS=darwin go vet ./internal/launch/`.

## 2026-07-20 — T4: cancellable transactional ops

- Cancellation invariant added to `docs/safety.md` (#6): cancel at any phase
  boundary ⇒ manifest `failed` (cause recorded) + automatic rollback to
  pre-op state + zero partial files + `errors.Is(err, context.Canceled)`.
- `internal/installer`: ctx checks pre-extract, post-extract (staging
  dropped), per-file in the swap loop, and pre-commit. `cancelInstall`
  marks failed then rolls back under `context.WithoutCancel` — cleanup is
  part of the atomic op, so it must finish even with a dead op ctx
  (WithoutCancel over Background to keep caller values). Leftover-manifest
  rollback on Install entry likewise detached. `Uninstall` gained per-entry
  ctx checks and persists progress on cancel (processed entries dropped,
  unprocessed retained, manifest stays committed) so retry resumes.
- `internal/app`: pre-resolve and pre-download ctx checks; new `ops.go`
  (`Op`, `RunOps`) — errgroup with per-op derived ctx, first error wins with
  game-dir context, siblings cancelled. Added `golang.org/x/sync` (vendored,
  zero transitive deps).
- `internal/ui`: per-game cancellation — mutex-guarded
  `map[gameDir]context.CancelFunc`; doInstall/doUninstall/Rollback derive
  cancellable ctxs; exported `Session.CancelOp(gameDir) bool`; new
  `EvOpCancelled`; cancel settles with row back to pre-op status, one
  "Cancelled" toast/event, no failure spam. Op slot released before
  terminal events (no stale-slot race on rapid re-trigger).
- TDD: six new tests red-first (installer ×3, app ×3 incl. RunOps, ui ×1),
  then green; five pre-existing fault-injection tests untouched and passing.

## 2026-07-20 — W3-T3: cross-platform multi-store discovery

- TDD throughout: every parser/probe/orchestrator behaviour landed as a
  failing test first (Epic fixtures, GOG playTasks, recursive depth/heuristics,
  rescan idempotence, fake-registry GOG, compat prefix, Store enum, ScanAll
  merge/dedupe), then implemented to green.
- `internal/domain`: additive `Store` enum (`StoreSteam` is the zero value so
  pre-existing Games stay Steam), `Game.Store/AppName/ExePath/CompatPrefix`.
- `internal/discovery` split into OS-agnostic parsers vs build-tagged probes:
  - Parsers (test on every GOOS): `epic.go` (.item manifests, `games`
    category filter), `gog.go` (goggame-<id>.info playTasks → primary exe,
    Windows separators normalised), `recursive.go` (each subdir of a root is
    a game; exe search depth ≤ 3; skip-token list; rank = folder-name match
    → size → 64-bit name → lexicographic; canonical-path dedupe),
    `gog_registry.go` (registry-reader seam), `plist.go` (XML plist via
    encoding/xml only; binary plists rejected), `scanall.go` (`ScanAll`
    merges Steam → Epic → GOG → apps → manual, deduped by canonical
    InstallDir, ctx-aware).
  - Probes (GOOS-tagged): Steam roots (linux paths moved to `steam_linux.go`
    unchanged; Windows HKLM `SOFTWARE\Valve\Steam` InstallPath with WOW64
    fallback; macOS `~/Library/Application Support/Steam`), Epic manifest
    dirs (Windows `%ProgramData%`, macOS `~/Library`), GOG registry
    (Windows-only; `golang.org/x/sys/windows/registry` — vendored that
    subtree only via `go mod vendor`), macOS `/Applications` .app scan
    (XML Info.plist → CFBundleName/CFBundleExecutable, else bundle-name
    fallback), `compat_linux.go` Proton prefix
    (`steamapps/compatdata/<appid>/pfx`, display-only) with stub elsewhere.
  - Recursive binary acceptance is per-GOOS: unix = exec bit, skips
    .so/.dll/.desktop; Windows = .exe only; darwin = Mach-O magic or .app.
- Exported signatures `ScanSteam`, `ParseLibraryFolders`, `ParseAppmanifest`,
  `ResolveInstallDir` unchanged; existing linux Steam tests untouched green.
- Verified: `go test ./...` (16 packages ok), `go vet ./...`,
  `golangci-lint run` (0 issues), `GOOS=windows`/`GOOS=darwin`
  `go vet` + `go test -c` compile green for discovery+domain. Deviation:
  cross-GOOS test binaries cannot execute on this host (no wine/darling;
  binfmt_misc registers only mono CLR) — compile-verified instead.

## 2026-07-20 — W4-T5: multi-store scan with per-game versions

- TDD: three app tests (multi-store scan via `ScanAll`, component versions
  from fixture PE DLLs, OptiScaler manifest/log/ini version chain), two ui
  row tests (platform from store, compat prefix + versions on rows), and the
  cmd scan golden test extended for the store/version columns — all red
  first, then green.
- `internal/app`: additive `ScanAllLibraries(ctx, st, ScanAllOptions{
  SteamRoot, ExtraDirs})` over `discovery.ScanAll` (SteamRoots from the
  steamRoot override, RecursiveRoots from settings ExtraDirs);
  `ScanLibrary` kept unchanged for existing callers. `LibraryEntry` gains
  `OptiScalerVersion` + `ComponentVersions` ("dlss"/"fsr"/"xess" → marketing
  name). `ManualEntry` now tags games `StoreManual`.
- Version enrichment is guarded to managed installs (committed manifest or
  `OptiScaler.dll` in the resolved injection dir) — no PE parsing for
  unmanaged games. Per-DLL: `classify.DirFiles` (new; full paths, reuses the
  existing classification table) → `pever.FileVersion` →
  `pever.MarketingName`; parse errors skip the component at debug level.
- `internal/ui`: `GameRow` gains `Store/AppName/ExePath/CompatPrefix`
  (T6 launch consumes these) plus `OptiScalerVersion` and sorted
  `Components`; `Platform` now derives from `Game.Store.String()` (hardcoded
  "Steam" removed). `Session.Scan` runs the multi-store scan with settings
  ExtraDirs as recursive roots.
- CLI `scan`: one line per game, now with store and a versions column
  (`OptiScaler 0.9.4, DLSS 3.7.10` or `-`); reads ExtraDirs from settings.
- New `internal/testutil` package: shared synthetic-PE fixture builder used
  by app and cmd tests (no production dependency).
- Verified: `go test ./...` (18 packages ok), `go vet ./...`,
  `golangci-lint run` (0 issues).

## 2026-07-20 — W4-T6: session game launching with user template

- TDD: five red-first tests — `TestLaunchTemplatePersists` (settings:
  default, custom round-trip, empty normalization) and four ui tests
  (`TestSessionLaunchSteamBuildsRunGameID`, `TestSessionLaunchManualUsesTemplate`,
  `TestSessionLaunchUnknownStoreErrors`, `TestSessionLaunchNotifiesOnSpawnFailure`)
  failing on undefined `LaunchTemplate`/`Deps.Launcher`/`Session.Launch`
  before the implementation.
- `internal/settings`: additive `LaunchTemplate` field (default
  `"{exe}" {args}` via exported `DefaultLaunchTemplate`); empty in JSON
  normalizes to the default at load and save, mirroring `DefaultVersion`.
- `internal/ui`: `Session.Launch(gameDir)` — fire-and-forget goroutine (no
  op slot; a spawn request is instantaneous and proves nothing about the
  game running). Row → `launch.Target` mapping (domain store → launch
  store; manual games get `Settings.LaunchTemplate`). Blank `ExePath` on
  manual/GOG rows falls back to `discovery.ScanRecursive` exe picking on
  the parent dir (canonical-path match); unresolvable → clear error toast,
  no launch. Success: `Launch requested: <game>` toast + `EvOpDone`;
  failure: warn toast `Launch failed: <err>` + `EvOpFailed`. zerolog info
  "launch requested" with store/game/argv0. `SetLaunchTemplate` persists
  like `SetDefaultVersion`.
- Runner seam: `Deps.Launcher *launch.Launcher`; nil selects the platform
  detached-spawn default (`launch.New(nil, "", nil)`). Tests inject a
  capturing runner + stub lookPath; captured argv artifacts:
  `[xdg-open steam://rungameid/100]` (Steam), `[umu-run <exe> --]` (custom
  manual template).
- Verified: `go test ./...` (18 packages ok), `go vet ./...`,
  `golangci-lint run` (0 issues). No changes to launch/discovery/domain.

## 2026-07-20 — W5-T9: bubbletea TUI frontend on ui.Session

- TDD: six red-first teatest tests in `internal/tui/model_test.go` failing on
  undefined `New`/`Model` before the implementation: `TestTUIListsGames`
  (golden-ish frame assertion on the final view + captured frame artifact),
  `TestTUIFilter` (`/` narrows, Esc clears via `Session.SetQuery("")`),
  `TestTUIInstallRoundTrip` (enter → real install → status line),
  `TestTUIQuitsOnQ`, `TestTUIConfirmEACPrompt` (EAC dialog, `y` consents,
  install proceeds), `TestTUILaunchBinding` (`l` → `Session.Launch` via the
  injected launcher seam; captured argv `[xdg-open steam://rungameid/100]`).
- `internal/tui`: second frontend proving the frontend-agnostic core.
  bubbletea `Model` over `*ui.Session` — the classic channel→Cmd bridge
  (`waitEvent` re-subscribes after every event), render from
  `Snapshot()`/`VisibleRows()`, keys forwarded to session commands
  (j/k navigate, enter quick install/uninstall, l launch, c cancel,
  `/` filter, `?`/f1 help, q quit). Pending confirmations are modal
  (`y`/`n`/Esc → `AnswerConfirm`). Plain-text rendering; no lipgloss/bubbles
  in production code. Zero business logic.
- `cmd`: shared session construction factored out of `gui.go` into
  `cmd/session.go` (`newSession`), consumed by both frontends; new `tui`
  kong subcommand (GUI stays the default). zerolog is redirected to
  `$OM_DATA_DIR/tui.log` while the TUI runs so logging never corrupts the
  display (`zerolog.Nop()` fallback).
- Test env mirrors the ui seams through the exported API only: httptest
  GitHub/CDN/search, temp store, temp Steam root, `Deps.Launcher` capture
  runner; hermetic (no home dir, no network). teatest note: `WaitFor`
  consumes the output stream — one wait per synchronization point.
- Deps: `github.com/charmbracelet/bubbletea v1.3.10`,
  `github.com/charmbracelet/x/exp/teatest v0.0.0-20260719004043-bb9a97036f23`
  (pins bubbletea v1 API); lipgloss/x-ansi/cellbuf etc. enter `vendor/` only
  as transitive test-deps of teatest. `go mod tidy && go mod vendor` in this
  commit.
- Verified: `go test ./...` (19 packages ok, 6 new tui tests), `go vet ./...`,
  `golangci-lint run` (0 issues), `goreleaser release --snapshot --clean`.
  No changes to `internal/ui`'s public API.
## 2026-07-20 — W5-T7: GUI keyboard nav, exit, responsive grid, version badges, launch

- TDD: eight red-first tests (compile-red on the new seams, then green):
  `TestFocusableButtonTabCyclesAndEnterActivates`,
  `TestFocusableButtonConsumesKey`, `TestExitButtonFlushesSettings`,
  `TestRenderPNG800pxValid`, `TestRenderPNG3840pxValid`,
  `TestCardShowsVersionBadges`, `TestLaunchButtonCallsSessionLaunch`,
  `TestEscClosesModal` (EAC modal regression).
- `internal/gui/widgets.go` (new): `focusableButton` — wraps
  `widgets.Button` in a `Focusable` container with `CycleFocusOnTab`, a
  focus ring, and Enter/Space activation that consumes the key
  (`FrameInput.Key = KeyCodeNone`) so no later widget double-fires. Applied
  to every button: cards, toolbar, sidebar, settings/about/confirm/dashboard
  modals. Tab and Shift-Tab cycling across the whole app comes free from
  shirei's global focus cycle. Also `versionPills` (✦ OptiScaler pill
  versioned when known, one pill per component version, Proton marker when
  `CompatPrefix` is set) and `launchable` (AppID or ExePath).
- Exit: sidebar "Exit" calls `model.exit()` — flushes a pending
  settings-modal edit through the session's existing persistence path
  (`SetDefaultVersion` → `settings.Save`), then quits via the injected
  `exitNow` seam (`os.Exit` in production; shirei has no `app.Quit`).
- Launch: per-card and dashboard "Launch" button (only when launchable) →
  `session.Launch(dir)`; dashboard busy state gains a "Cancel" button →
  `session.CancelOp(dir)`. Cards and dashboard render the version pills.
- Responsive grid: `fitCards` clamps columns to [1, 8], caps card width at
  320px (rows stay left-aligned on ultrawide), and computes card width from
  the padding-corrected inner width — fixing a ~22px right-edge clip the old
  math produced at every window size. shirei API verification notes: key
  constants are `KeyEnter`/`KeySpace`/`KeyCodeNone` (not `KeyCodeEnter`/
  `KeyCodeSpace`); `CycleFocusOnTab`, `HasFocus`, `Focusable`, and the
  modal's built-in Esc handling confirmed against vendored v0.5.2.
- Render artifacts (validated in-test via the existing RenderToPNG harness):
  800x600 frame decodes at 800x600 with cols=3, cardW=230, no horizontal
  overflow; 3840x1080 frame decodes at 3840x1080 with cols capped at 8 and
  cardW capped at 320.
- Verified: `go test ./...` (18 packages ok), `go vet ./...`,
  `golangci-lint run` (0 issues). No changes outside internal/gui + this log.

## 2026-07-20 — W6-T8 (F1): zero-config first-run E2E + empty states

- TDD: four red-first tests. RED evidence: `TestFirstRunEmptyLibraryScanSucceeds`
  failed with `scan reported as failure: "no games found"`;
  `TestEmptyStateCopyShown` compile-red on the missing `emptyStateCopy` seam.
  `TestFirstRunZeroConfigScanToInstalled` and `TestFirstRunEACInlinePrompt`
  passed on first run — they prove the existing flow end-to-end (fresh
  profile, fake GitHub, real bundle.7z fixture): scan → ONE QuickInstall →
  status committed + `dxgi.dll` in the game bin dir; EAC game → inline
  confirm pending with NO install started → `AnswerConfirm(true)` →
  installed. Full `t.Log` transcripts in `internal/ui/firstrun_test.go`.
- Friction fix (the only genuine first-run blocker found):
  `app.ScanAllLibraries` errors with "no games found" on an empty library,
  so a first-run user with zero games got a scary `EvScanFailed` ("Scan
  failed: no games found") and no guidance. `internal/ui/session.go` now
  settles that one case as a successful empty scan (`EvScanDone "0 games"`);
  all other scan errors still fail loudly. The message is matched via the
  new `emptyLibraryError` const because internal/app is read-only here — it
  should export a sentinel later.
- `internal/gui/view.go`: `emptyStateCopy(query)` + `emptyState()` — one
  muted line in place of an empty grid/list: "No games found — use Add Game
  to register a folder" (empty library) vs "No games match …" (filter).
  Wired into `gridView` and `actionList`; audit table untouched (dev view).
- Friction audit, checked and NOT changed: settings modal is never required
  before install (`DefaultVersion` defaults to `latest`); EAC and
  stale-cache consent gates still fire (proven by tests); TUI keeps its
  "(no matches)" line (conflates empty library with filter-empty, but
  harmless and ponytail-minimal); "no Steam installation found" only exists
  in the CLI `ScanLibrary` path — the session's multi-store scan never hits
  it, and the empty-library fix covers the GUI/TUI outcome.
- Verified: `go test ./...` (20 packages ok), `go vet ./...`,
  `golangci-lint run` (0 issues). No changes outside internal/ui,
  internal/gui, and this log.

## 2026-07-20 — W6-T10: v0.3 release matrix, CI cross-GOOS gates, docs wrap

- v0.3 wraps W3–W5: `internal/pever` (PE version parser + marketing maps),
  `internal/launch` (per-store per-OS command table, detached spawn, never
  `proton run`), cross-platform multi-store discovery (W3), multi-store scan
  with per-game versions on rows (W4-T5), session launching with the user
  template (W4-T6), the bubbletea TUI (W5-T9), and GUI keyboard nav / exit /
  responsive grid / version badges / launch (W5-T7).
- **goreleaser**: snapshot run green — artifacts
  `optiscaler-manager_v0.0.0_linux_amd64.tar.gz` and
  `optiscaler-manager_v0.0.0_windows_amd64.zip` (plus `sha256sums.txt`).
  darwin amd64+arm64 stays disabled after an honest diagnosis, recorded in
  `.goreleaser.yml`: shirei v0.5.2's only macOS backend (vendored
  cocoabackend) is cgo + Cocoa/QuartzCore/IOSurface; `CGO_ENABLED=0` fails at
  compile (perf_darwin.go references symbols from the dropped cgo file) and
  cross-linking cgo from Linux needs an Apple SDK (no osxcross on the host;
  ubuntu-latest identical). Unlock path: macos-latest release runner. No
  macOS support is claimed anywhere in the docs.
- **CI**: new `xplatform` job — `GOOS=windows go vet ./...` (whole tree,
  CGO-free), `GOOS=darwin go vet` + `GOOS=windows/darwin go test -c` scoped
  to `internal/discovery` and `internal/launch`. Cross-compiled test binaries
  cannot execute on Linux runners (no wine/darling), so the gate is
  compile-only — the W3 decision, now enforced in CI. Existing linux
  test/vet/lint/goreleaser jobs unchanged.
- **Docs**: architecture.md gains the v0.3 package map (pever, launch, tui,
  cmd/session.go, app ops.go, Store enum), a cross-platform-shape section,
  and the cancellation model; scope.md gains the v0.3 section (multi-store,
  versions, launch, TUI, cancel, GUI polish) with known limits (compile-gated
  cross-GOOS, no macOS artifacts, Epic AppName requirement, fire-and-forget
  launch); safety.md keeps cancel invariant #6 and gains a launch-safety
  section (detached spawn, shell-free template split, 10s URL-opener cap);
  README updated to v0.3 (usage, launch keys, template location, status).
  OKF frontmatter untouched; `TestDocsOKFFrontmatter` green.
- Verified: `go test ./...`, `go vet ./...`, `GOOS=windows go vet ./...`,
  `GOOS=darwin go vet` + `go test -c` (discovery, launch), `goreleaser
  release --snapshot --clean`. No Go source changed; no new dependencies.

## 2026-07-20 — fix(pever): raw-keyed FSR version map, bounded reads, real subcomponent coverage

- **Root cause (review-blocking)**: `internal/pever/versionmaps.go` keyed
  the FSR table by marketing numbers ("3.1.4", "4.1.1"), but real FSR DLLs
  report raw PE file versions (FSR 3.1.4 = `1.0.1.41314`, FSR 4.1 =
  `2.2.0.1328`), so tier-2 lookup mislabeled them "FSR 1.0"/"FSR 2.2".
- **Fix**: all three tables re-vendored VERBATIM (keys) from the reference
  client Agustinm28/Optiscaler-Client `assets/configs/{fsr,dlss,xess}_version_map.json`
  (main == development, fetched 2026-07-20): FSR 22 entries, DLSS 23
  (adds 310.4.0→4.0, 310.5.x/310.6.0→4.5), XeSS 10. Values keep the
  package's vendor-prefixed output convention. The 4-tier algorithm is
  unchanged; numeric compare handles 4-segment keys.
- **TDD**: real-raw cases added to `TestMarketingNameLookup_FourTier` red
  first (`MarketingName(FSR,"1.0.1.41314") = "FSR 1.0", want "FSR 3.1.4"`),
  then green via the new tables. Stale expectations updated to real data
  (old data was wrong). Table-driven tier-3 case dropped: no cross-major
  same-value bracket exists in the real maps; tier-3 stays covered by the
  synthetic `lookupMarketing` subtests.
- **Hardening (security MEDIUM)**: new `readBounded` — stat first, require
  regular file (`ErrNotRegular`), size cap (`ErrTooLarge`), read via
  `io.LimitReader`. `FileVersion` caps at 128 MiB (deviation from the
  suggested 64 MiB: the real 0.9.4 bundle ships a 78 MB libxess.dll);
  `manifestVersion` caps at 1 MiB. Red-first tests: >cap sparse file,
  directory-as-PE, oversized manifest falls through to the log chain.
- **Real-bundle proof** (`OM_TEST_ARCHIVE` gated, extended): 0.9.4
  subcomponents — vk.dll `1.0.1.41314`→FSR 3.1.4, upscaler `4.1.1.0`→FSR
  4.1, framegen `4.0.1.0`→FSR 4.1, dx12 shim `2.3.0.0`→FSR 4.1 (tier-2
  fallback; reference map has no 2.3.x key), libxess `2.0.2.68`→XeSS 2.0;
  no nvngx_dlss.dll ships in 0.9.4 (conditional check catches future drift).
- **Blast radius outside pever (deviation)**: `cmd/cli_test.go` and
  `internal/app/scan_test.go` fixtures used marketing-style synthetic
  versions (3.7.10.0 / 3.1.4.0) that only resolved under the old wrong
  table; updated to REAL raws (3.7.20.0→DLSS 3.7.20, 1.0.1.41314→FSR 3.1.4).
- Verified: `go test ./...`, `go vet ./...`, `golangci-lint run` (0 issues),
  full suite with `OM_TEST_ARCHIVE=/tmp/opencode/optiscaler-0.9.4.7z`.

## 2026-07-20 — v0.4 W1 (T1): games cache, Session.Start, RemoveDirectory, SetSort

- `internal/ui/cache.go` (new): the games-list cache, `games.json` in the
  data root, schema-versioned envelope (`version: 1`), atomic temp-write +
  rename. `loadGamesCache` yields nil (never an error) on missing,
  unreadable, corrupt, or stale-schema files so callers fall through to a
  real scan; `saveGamesCache` is best-effort (logs, never fails the
  caller).
- `Session.Start(ctx)`: cache-first boot. A warm cache hydrates rows
  synchronously and reconciles each row's status from store manifests
  (keyed by canonical install dir, falling back to game root) so installs
  that settled while the manager was not running show their real state.
  No PE parsing, no reclassification, no scan; status line
  `N games (cached)`. An empty/unusable cache falls through to `Scan`.
- `persistCache` rewrites the cache after scans, `AddDirectory`,
  `RemoveDirectory`, and op settles (`setRowStatus`), serialized under a
  dedicated mutex so concurrent scan/op writers cannot interleave.
- `Session.RemoveDirectory(dir)`: drops the directory's row and any nested
  games scanned under it, persists settings without it, rewrites the cache;
  directories not in ExtraDirs are a silent no-op. `Session.SetSort(mode)`
  selects `SortDefault` (actionable-first) or `SortName` (A–Z) in
  `VisibleRows`; out-of-range modes reset to default.
- TDD: cache round-trip/corrupt/stale-schema tests, Start warm/cold boot,
  status reconcile, RemoveDirectory row/settings/cache effects, SetSort
  ordering, red first, then green.

## 2026-07-20 — v0.4 W2a (T2): GUI polish, settings directories, cache-first boot

- Settings modal gains sections: "Scan Directories" lists `ExtraDirs` with
  per-row Remove buttons and an "Add directory…" entry that opens the OS
  picker (`Session.PickAndAddDirectory`); "Launch Template" edits the
  manual-game launch template in-app through `SetLaunchTemplate` (hand
  editing `settings.json` no longer required). Directory rows ellipsize at
  the front so the distinctive tail stays visible; the list is a bounded
  Viewport inside the auto-sized modal.
- Theme tokens: spacing, radii, elevation, and an expanded palette in
  `theme.go`; scrollbars and the toolbar search input themed dark (upstream
  widgets hardcode light surfaces).
- Cards and list rows gain hover states; games without cover art render a
  deterministic gradient placeholder (glyph + title initial, FNV-hashed
  hue) instead of the tiny dark no-art tile (`_placeholder.png` detected by
  filename).
- The dashboard modal is replaced by a right-docked detail side panel: the
  grid stays visible beside it; Close button and Esc both dismiss.
- Toolbar: sort menu (Default: actionable first / Name: A–Z) wired to
  `Session.SetSort`, icon grid/list view switch, scan spinner while busy;
  sort/view/search controls disable while the library is empty.
- Empty states: centered icon, heading, guidance, and CTA buttons
  (scan/add-directory on an empty library, clear-search on an empty
  filter).
- Chrome: 72px icon sidebar with active-section accent, status-bar shortcut
  hints (`Tab: focus · ←→↑↓: select · Enter: open · Esc: close`), raised
  toast cards with a tone accent bar capped at three.
- Arrow-key grid navigation: ±1 across, ±cols up/down; Enter opens the
  detail panel for the selected card, Esc closes it; global keys run last
  in the frame so focused widgets get first pick, and modals own their own
  keys.
- GUI boots cache-first via `Session.Start` (`model.boot`); a warm cache
  shows rows instantly, a cold cache falls through to a scan inside Start.
- Verified: `go test ./...`, `go vet ./...`; headless RenderToPNG smoke
  tests cover the new views (nav, hover, sort, toolbar, settings, empty
  states).

## 2026-07-20 — v0.4 W2b (T3): TUI overhaul — screens, styling, settings dirs, bubbles

- Multi-screen layout: number keys switch top-level screens (1 Games /
  2 Settings / 3 Help); per-screen update/view handlers on one flat model.
- Styled game columns (badges, title, store, version, status) with tone
  colors via lipgloss; resize-aware layout on bubbles viewport.
- Detail screen (`enter`) with actions `i` install / `l` launch /
  `c` cancel / `r` rollback / `o` open INI; `esc` back. Live `/` filter
  (Esc clears), `s` sort toggle, `R` rescan, `i` quick install on the games
  screen; `q` and `ctrl+c` quit.
- Settings screen: `e` edit default version, `t` edit launch template,
  `a` add a scan directory inline, `d` remove the selected one behind an
  inline `y`/`n` confirm, `x` clear the bundle cache.
- Centered confirm modal for session consent gates (`[y]` proceed /
  `[n]` cancel); unrelated keys are swallowed while it is up. Busy spinner
  (bubbles spinner), toasts, empty-state guidance.
- `Init` boots the library cache-first via `Session.Start` (warm cache
  hydrates rows without a scan; cold falls through to one).
- Deps: bubbles v1.0.0 added and vendored; lipgloss promoted to a direct
  dependency (`go mod tidy && go mod vendor` in the merge).
- TDD: teatest RED tests first for the multi-screen UI, settings directory
  management, and cache-first start, then green. Verified: `go test ./...`,
  `go vet ./...`, `golangci-lint run` (0 issues; one ineffassign fix in
  status cell styling).

## 2026-07-20 — fix(settings): Save is a no-op on empty root

- `settings.Save("")` previously attempted `MkdirAll("")` + temp-file
  creation and errored, surfacing as save-error toast storms in contexts
  without a state dir (tests, misconfigured launches). An empty root now
  returns nil immediately: sessions without a state dir must not fail or
  spam callers. Behavior for non-empty roots is unchanged (atomic temp +
  rename, defaults normalized).

## 2026-07-20 — v0.4 docs wrap

- README: status → v0.4, cache-first startup and `games.json` in the state
  root, in-app settings description (scan-directory add/remove, launch
  template edited in-app), GUI interaction summary, TUI keymap table.
- scope.md: v0.4 section (settings UI, games cache, GUI polish, TUI
  overhaul) with known limits (no GUI `/` focus shortcut, cached
  versions/covers until next scan, macOS still blocked).
- architecture.md: ui/cache.go in the package map and a startup-flow
  section (cache-first boot, manifest status reconcile, persist triggers).
- plan.md: v0.4 milestone recorded complete; index.md release line → v0.4.
- No Go source changed; OKF frontmatter untouched.

## v0.4 review gate (T5) — 2026-07-20

Five-lane review of the whole v0.4 range; two lanes failed round 1 and were
fixed forward to unconditional PASS:

- **R1 goal**: FAIL → PASS. Blocker: settings modal used shirei's hardcoded
  light TextInputExt in the dark UI. Fixed by extracting the shared
  `themedInput` widget (`869e409`).
- **R2 QA**: PASS. 30 scenarios (P0 14/14) incl. restart persistence,
  remove-dir-during-inflight-install, real-bundle `OM_TEST_ARCHIVE` run,
  GUI/TUI artifact inspection.
- **R3 quality**: FAIL → PASS. CRITICAL: `RemoveDirectory` `[:0]` in-place
  filter raced concurrent `Scan` (`-race`-proven) — fixed with deep-copied
  `Settings()` snapshots + allocation + `TestSessionRemoveDirectoryVsScan`
  (`c311571`). MAJOR: TUI `trunc` sliced ANSI escapes — fixed with
  badge-aware truncation (`c9f6af0`); `detailRow` double-snapshot fixed
  (`a4cd33f`).
- **R4 security**: PASS (severity NONE). Data root is the trust boundary; no
  shell invocation anywhere; 0600 temp-file perms; supply chain verified.
- **R5 context**: PASS. Docs/README/keymaps in sync; no missed requirements.

## 2026-07-20 — v0.5 W1: core — PE titles, exec-bit fix, async AddDirectory, scan progress

- Manual/recursive games get their title from PE version info: `ProductName`
  wins, then `FileDescription`, then the folder-name fallback. Windows exes
  now carry real titles even when scanned on Linux.
- Linux recursive scans accept `.exe` files without the execute bit
  (`discovery/recursive_unix.go`) — previously-missed games now appear.
- `Session.AddDirectory` is non-blocking: validate → persist settings →
  insert a placeholder row synchronously, then walk/classify/cover in a
  goroutine that replaces the placeholder. A duplicate add while one is in
  flight is rejected. `ClearBundleCache` is likewise async.
- Scan reports progress through `State.Progress{Phase, Done, Total}` with
  phases discover/enrich/covers/lookup and `EvScanProgress` events; both
  frontends render it.

## 2026-07-20 — v0.5 W2: enrichment — steam/protondb online lookups

- `internal/steam` (new): title → appid via
  `steamcommunity.com/actions/SearchApps`, no auth, 30-day TTL disk cache.
- `internal/protondb` (new): appid → tier via the protondb.com summaries
  API, 7-day TTL disk cache, 429 cooldown.
- The lookup scan phase resolves manual rows title → appid → tier; numeric
  appid (Steam-library) rows get the tier directly. Per-scan budget of 8
  lookups, silent degradation when offline. Rows gain `SteamAppID` and
  `ProtonTier`.
- Gated by `online_lookups` in settings.json (default true; legacy files
  without the key decode as enabled). `Session.SetOnlineLookups` persists
  the toggle. Cover fetches gained a 10s HTTP timeout.

## 2026-07-20 — v0.5 W3a: GUI — sidebar, click routing, panel, CSD, progress, tier, toggle

- Sidebar nav items are uniform width (Expand).
- Card buttons fire their action without opening the detail panel; only
  card-body clicks open details (click routing fix).
- Detail panel is proportional: 30% of the window width, clamped 300–480px
  (`detailPanelWidth` in theme.go).
- Dark Wayland CSD titlebar via a vendor patch at
  `vendor/go.hasen.dev/shirei/waylandbackend/waylanddecor_linux.go` (marker
  `PATCHED by optiscaler-manager (v0.5)`), guarded by
  `internal/gui/csd_test.go` (`TestVendorCSDPatchPresent`); procedure in
  `docs/vendor-patches.md`. Wayland-only; X11 keeps WM decorations.
- Scan progress bar under the toolbar (phase + Done/Total).
- ProtonDB tier pills (platinum/gold/silver/bronze/borked/pending) on cards
  and the detail panel.
- Settings gains the "Online game info (Steam/ProtonDB)" toggle.

## 2026-07-20 — v0.5 W3b: TUI — tab-bar root cause, About screen, hints, progress, tier

- **Root cause of "no access to Settings"**: the tab bar never rendered.
  View emitted h+1 lines, and bubbletea's renderer drops line 0 (the tab
  bar) on oversized frames. Fixed: View now emits exactly h lines.
- Every screen footer shows the screen-switch hints
  `1 games · 2 settings · 3 help · 4 about`.
- New About screen (key 4): build version plumbed from cmd plus the stack
  line `TUI: bubbletea v1.3.10 · bubbles v1.0.0 · lipgloss`.
- Escape hints in input modes and confirm modals; scan progress line
  (phase + bar + percent); ProtonDB tier in the games table badges and the
  detail screen; settings `o` toggles online game info.

## v0.5 review gate — 2026-07-20

- Review gate (5 lanes): PASS. R1 goal PASS; R2 QA PASS (34/34 scenarios
  incl. live-scan progress probes in both frontends); R3 quality FAIL→PASS
  (MAJOR: remove-during-add zombie row → CancelOp + ExtraDirs re-verify
  (0e4c3fa); MAJOR: lookup starvation → negative caching 7d/30d + budget
  counts live requests only (9c3957d, 63e5c96, 199d970); MINOR: TUI
  settings height clamp (ed72b1a)); R4 security PASS (LOW; appid digit
  validation + 1 MiB body caps applied); R5 context PASS.

## 2026-07-20 — v0.6 T1: pever.DetectOptiScaler + StringInfoPE fixture

- Merge `4246426` (`75fbd13` feat + `26bb971` test fixture).
- `internal/pever/detect.go`: `DetectOptiScaler(dir)` scans the
  injection-name candidates in order (dxgi.dll, OptiScaler.dll, winmm.dll,
  dbghelp.dll, version.dll, wininet.dll, winhttp.dll, d3d12.dll); a
  candidate matches when ProductName, CompanyName, or OriginalFilename
  contains "optiscaler" (case-insensitive). OriginalFilename survives
  renames, so a shim renamed to dxgi.dll still identifies; non-matching
  candidates (e.g. a DXVK dxgi.dll), non-PE files, oversized files, and
  unreadable paths are skipped without stopping the scan.
- Version evidence chain: OptiScaler's own `manifest.json` →
  `OptiScaler.log` banner → the matched candidate's PE version resource
  (one `resourceBytes` pass serves identity and the PE fallback) →
  "" (installed, version unknown). Reads bounded via `ReadBounded`.
- `internal/testutil`: `StringInfoPE` synthetic-PE fixture builder mints
  branded DLLs for tests across packages (no production dependency).
- TDD red→green throughout.

## 2026-07-20 — v0.6 T2: domain.StatusExternal

- Commit `1684c73`.
- `internal/domain/manifest.go`: `StatusExternal Status = "external"` —
  marks an OptiScaler installation detected on disk that this manager did
  not perform. Derived at scan time and NEVER persisted to store manifests;
  the persisted state machine stays the four statuses (in_progress,
  committed, failed, rolled_back). Round-trip test pins the persisted set.

## 2026-07-20 — v0.6 T3: app — external detection in enrich + ErrNotManaged

- Merge `db49233` (`b2c6cc3` enrich probe + `da5301f` sentinel).
- `internal/app`: unmanaged rows (no store manifest, resolvable injection
  dir) are probed with `pever.DetectOptiScaler` during scan enrich — the
  probe runs inside the scan goroutine with bounded reads; manifests stay
  authoritative for managed games. Matches get `StatusExternal` and the
  chain-recovered version. `enrichVersions` skips external rows: component
  versions are suppressed (those DLLs belong to OptiScaler's bundle, not
  the game).
- New sentinel `app.ErrNotManaged` for uninstall/rollback against a game
  the store holds no manifest for — the raw store error must never reach a
  toast (consumed by the session in T4).

## 2026-07-20 — v0.6 T4: ui — external adopt/refuse/re-detect flows + CanOpenINI

- Merge `0efe1b4` (`9d89fdf` CanOpenINI + external test suite, `d91fd1f`
  refuse uninstall, `8fa6593` re-detect after uninstall, `68a2f18`
  warm-cache reconcile lock). Keystone SHAs: `9d89fdf` (suite + keystone
  test) and `8fa6593` (post-uninstall re-detect completing the round trip).
- `GameRow.CanOpenINI()` (new predicate): committed or external — external
  installs carry a real on-disk OptiScaler.ini.
- Uninstall of a never-managed external row is refused up front with the
  clean toast "not installed by this manager — adopt first or remove
  manually": no op registered, store untouched, external files exactly as
  found, no raw sentinel leak. The same toast covers the manifest-vanished
  race (`app.ErrNotManaged`).
- After a managed uninstall, `pever.DetectOptiScaler` re-runs on the row's
  injection dir: a restored external install settles the row back to
  external instead of bare uninstalled.
- Warm-cache reconcile keeps cached external rows: manifests override only
  where they exist, so `games.json` carries the derived status until the
  next rescan.
- **Keystone** (`TestAdoptRoundTripRestoresExternalBytes`): external marker
  dxgi.dll → scan shows external → QuickInstall adopts (SHA-verified backup,
  committed, bundle dxgi.dll) → uninstall restores the marker bytes
  byte-identically (SHA-256 compared) → re-detect shows external again.

## 2026-07-20 — v0.6 T5: GUI — external status rendering

- Merge `da6cd16` (`8ceb4ac`).
- Quick action caption on external rows reads "Adopt" (`quickLabel`;
  committed stays Uninstall). Status pill "external" in blue via
  `statusTone`; version pills render "✦ OptiScaler <v> · external" (blue),
  or "✦ OptiScaler · external" when the version is unknown. Badge pills
  shared by list rows and grid cards via `statusPill`.
- Detail panel OpenINI gated on `GameRow.CanOpenINI` (committed or
  external).
- TDD: RED captured per-test (compile-error scaffolding stage), then GREEN;
  `internal/gui/external_test.go` (177 lines).

## 2026-07-20 — v0.6 T6: TUI — external-install status rendering

- Merge `9cfb942` (`3e5f08b`).
- `gameRowLine`: explicit `external` case wearing a new `styleInfo` accent
  (bright cyan) — distinct from committed green, warn red, busy yellow, and
  muted gray.
- Detail view: open INI gating now uses `GameRow.CanOpenINI()` (committed or
  external) instead of the `"committed"` string literal; external rows show
  the adopt hint `i  adopt (install over external)` instead of
  `install/uninstall`.
- Detail `o` key gates on `CanOpenINI()`, so external installs open their
  on-disk `OptiScaler.ini` from the TUI.
- TDD red→green: `TestGameRowLineExternalStatus`,
  `TestDetailViewOpenININotDimmedForExternal`,
  `TestDetailViewAdoptHintForExternal`,
  `TestDetailKeyOpenINIAllowedForExternal` (opener seam probed with a fake
  `xdg-open` earlier in PATH).

## 2026-07-20 — v0.6 T7: CLI — external scan output characterization

- Merge `5ba7444` (`6f01f0b`).
- `TestScanCommandShowsExternalInstall` pins the CLI scan rendering of an
  unmanaged OptiScaler install: an OptiScaler-branded dxgi.dll with no
  manager manifest surfaces as `[external]` with the PE FileVersion in the
  versions column. Characterization test — `cmd/scan.go` already renders any
  non-empty status and OptiScalerVersion; no production change.

## 2026-07-20 — v0.6 T8: docs wrap

- README: status → v0.6 (external detection feature paragraph), scanning
  section gains the external-detection rule (PE-identity probe, async in the
  scan goroutine, bounded reads, unmanaged-only, component versions
  suppressed, cached until rescan), usage notes gain Adopt-vs-Install
  semantics, the never-managed uninstall refusal, and adopt
  backup/restore behavior.
- scope.md: v0.6 section (detection rule + evidence chain, bounded/
  unmanaged-only/async probe, derived status, adopt/refuse/restore) with
  known limits (PE-branded DLL required — ini/log remnants alone don't
  count; status cached until rescan; injection dir must resolve; component
  versions suppressed).
- architecture.md: pever/domain package-map lines, and an external-install
  detection section (enrich-phase probe, 4 persisted + 1 derived status
  model, adopt/restore/re-detect session semantics).
- plan.md: v0.6 milestone recorded complete (T1–T9); index.md release line
  → v0.6. No Go source changed; OKF frontmatter untouched.

## v0.6 review gate (T9) — 2026-07-21

Five-lane review of the v0.6 range; quality failed twice and was fixed
forward to unconditional PASS:

- **R1 goal**: PASS (WARN: ManualEntry never probed → fixed in round 1).
- **R2 QA**: PASS (20/20 scenarios; strongest proof: the real 0.9.4
  OptiScaler.dll detected as external under all 8 injection names, version
  "0.9.4"; keystone adopt→uninstall byte-identical restore re-run).
- **R3 quality**: FAIL→FAIL→PASS. Round 1 MAJOR: rollback path missing
  post-restore re-detect → `redetectExternal` shared helper + rollback
  ErrNotManaged mapping (2c4012f); R1 WARN: ManualEntry probe (4533e51).
  Round 2 MAJORs: rolled_back manifest re-asserted on rescan/restart →
  manifest deleted when rollback restores external (798f03f); ManualEntry
  probed without store precedence, mislabeling managed manual games as
  external and blocking their uninstall → manifest precedence in
  ManualEntry(dir, st) (00008e4). Re-review verified both with independent
  traces (rescan + warm-cache stay external; managed manual stays
  committed + uninstall works).
- **R4 security**: PASS (LOW; subslice retention + ReadBounded TOCTOU
  accepted as bounded).
- **R5 context**: PASS (real-bytes identity verified; candidate list
  matches the official wiki; rollback divergence flagged — fixed in round
  1; OptiScaler.asi/nvngx.dll noted as unprobed edge deployment modes).


## 2026-07-20 — v0.7 T1: discovery — game-dir vs container predicate

- Merge `5201a42` (`c130c3d`, `283529d`).
- `discovery.ClassifyGameDir(ctx, dir) (GameDirKind, error)` sorts a
  directory into `GameDirGame` / `GameDirContainer` / `GameDirEmpty` using
  only stats and bounded walks (no PE parsing); candidacy, skip tokens, and
  the depth cap are exactly `findMainExe`'s. `LooksLikeGameDir` is the
  boolean form. Rules: exe at depth ≤ 1 → game; no gamey children → empty;
  exactly one gamey child with the exe within depth ≤ 2 (engine layouts) →
  game; otherwise → container.
- `ScanRecursive` skips exe-less subdirectories instead of surfacing
  phantom rows (`283529d`); `TestScanRecursive_InstallerNamesStillRejected`
  was replaced by `TestScanRecursive_InstallerOnlySubdirSkipped` (the old
  test characterized the phantom-row bug).
- Deviations recorded in the T1 log: kind-returning classifier instead of a
  bare bool; engine-edge override at depth ≤ 2 (deeper single-chain game
  dirs classify as container — documented in the docstring).

## 2026-07-21 — v0.7 T2: session — container scan roots + title pins

- Commits `8f9cc95` (scan gate), `7c64f30` (AddDirectory branch),
  `5d3587f` (title-priority pins); docs in the follow-up docs commit.
- Scan gate: extra roots are classified once per scan; container/empty
  roots get no `ManualEntry` self-row from `mergeExtraDirs`, are excluded
  from the covers progress total, and the in-flight merge drops stale
  container self-rows left in `games.json` by pre-gating builds. Roots
  that fail classification keep the previous row-bearing behavior.
  RED→GREEN: `TestMergeExtraDirs_SkipsContainer`,
  `TestScan_StaleContainerRowNotResurrected` (both red first);
  `TestMergeExtraDirs_GameDirRowKept` pins the game-dir path.
- `AddDirectory` classifies synchronously (bounded stats/walks, no PE
  parsing — cheap for an explicit user action; sync-classify decision
  documented in scope/architecture) and branches: game → v0.5 async
  contract unchanged; container → persisted as a scan root, no
  placeholder/self-row, "registered `<base>` as a scan folder" toast,
  background rescan surfaces the children (no "directory added" text —
  `race_test.go`'s scan-done filter stays compatible); empty → refused
  with a warning toast before any settings mutation. RED→GREEN:
  `TestAddDirectory_Container_NoSelfRow_ScanFolderToast`,
  `TestAddDirectory_Container_ChildrenSurfacedAfterRescan`,
  `TestAddDirectory_NoGamesAnywhere_Refused_NotPersisted` (red first);
  `TestAddDirectory_GameDir_RowAppears` pins the game path.
- Title-priority pins (characterization; behavior shipped in v0.5/v0.6):
  `TestManualName_PEProductNameBeatsFolder`,
  `TestManualName_FolderFallbackWhenNoPETitle`,
  `TestAddDirectory_PlaceholderReplacedByPETitle`.
- Deviation: three frontend test fixtures (`internal/gui` sort/nav,
  `internal/tui` settings) added genuinely EMPTY directories and expected
  rows/registration; under the T2 contract empty dirs are refused, so the
  fixtures gained a real `game.exe` (test-only, intent unchanged). The
  nav-test fix rode the scan-gate commit (the gate alone made it flaky:
  placeholder rows for empty dirs are dropped at scan).
- Docs: README scanning/add-directory sections, scope.md v0.7 + known
  limits, architecture.md classification section + package map, plan.md
  v0.7 milestone, index.md release line.

## 2026-07-21 — v0.7 T3: review-gate fix-forward (scan serialization, cache v2, symlink classify, TUI freeze)

- `Session.Scan` serializes: a `scanning` flag + `scanPending` bit replace
  the goroutine-per-call shape. A container added mid-scan previously raced
  the in-flight scan — the rescan surfaced the children, then the earlier
  scan settled last and its keep-block wiped them until the next manual
  rescan (R3 MAJOR). A Scan landing mid-scan now sets the pending bit (no
  goroutine, no events); the running scan re-runs once on settle, success
  or failure, on a fresh snapshot. Only scans that actually run emit
  EvScanStarted/EvScanDone. RED→GREEN:
  `TestScanSerialization_ContainerAddDuringBootScan` (red: children wiped),
  `TestScanSerialization_PendingRunsOnce` (red: 3/3 starts). `race_test.go`
  moves from exact completion counting (nondeterministic under coalescing)
  to a marker-scan liveness assertion + `scanIdle` quiescence.
- Games-cache schema bumped to v2: v0.6 (v1) caches carry stale container
  self-rows and now load as empty, so the first v0.7 boot falls through to
  a real scan (R1). RED→GREEN:
  `TestStart_StaleSchemaCacheFallsThroughToScan` (red: warm boot accepted
  the v1 cache). Frontend test fixtures mirroring the schema bumped
  (`internal/tui` seedGamesCache, `internal/gui` boot cacheJSON —
  test-only literals).
- `gameyChildren` resolves a symlink child with `EvalSymlinks` before the
  per-child walk: WalkDir never descends a symlink root, so symlinked game
  subdirs counted as non-gamey (parent misclassified empty) while
  ScanRecursive, canonicalizing first, scanned them (R4). RED→GREEN:
  `TestClassifyGameDir_SymlinkedGameChild`,
  `TestClassifyGameDir_SingleSymlinkedChild`.
- TUI add-dir commit returns a `tea.Cmd` instead of calling
  `AddDirectory` inline on the bubbletea update loop (session
  classification is synchronous) (R4). Structural RED (compile) → GREEN:
  `TestCommitInputAddDirDeferred`.
- Docs: scope.md v0.7 known limits (depth-2/3 self-row boundary, nested
  containers surfacing one game per intermediate dir per scan, v1 cache
  invalidation); ScanRecursive documents exe-less-subdir skipping;
  depthOf documents depth 1 as an immediate subdirectory.

## v0.7 review gate (T3) — 2026-07-21

Five-lane review of the v0.7 range; quality failed round 1 and was fixed
forward to unconditional PASS:

- **R1 goal**: PASS (WARNs: warm-cache stale rows → cache schema v2;
  nested-container collapse → documented).
- **R2 QA**: PASS (17/17 scenarios; deviations pinned as documented
  semantics: nested containers surface the intermediate dir, bin-layout
  phantom child rows pre-exist, depth-1 rule).
- **R3 quality**: FAIL→PASS. MAJOR: container-add rescan raced in-flight
  scans — a stale scan settling last wiped freshly surfaced container
  children (keep-block only covers ExtraDir self-rows) → scan
  serialization with a pending bit (e88e056; deterministic gated-CDN
  regression test). Also fixed in the wave: cache schema v2 (d8d437f),
  symlink-child canonicalization (f1f39a8), TUI add-dir off the update
  loop (5e3f0cf), docs (0dc5bb3).
- **R4 security**: PASS (LOWs resolved in the same wave).
- **R5 context**: PASS (reference client is also exe-based; no convention
  contradicted).

## v0.7.1 container recursion fix — 2026-07-21

User report after v0.7: containers (e.g. `Games`, `Steam`) still appeared
as rows and games were lost. Root cause: the v0.7 gate classified only the
top-level added directory; `ScanRecursive` still rowed any child with an
exe within depth 3, so a nested container became a row that stole one
child's PE title while its siblings were dropped.

- `9875f5e` fix(discovery): containers are transparent at every scan
  level. `ScanRecursive` classifies the root and every child
  (game/container/empty): game dirs yield their own row plus one per game
  child, containers are recursed into transparently (never rows), engine
  folders (`bin`, `Binaries`, `Win64`, `x64`, …) never row.
  `ClassifyGameDir` is now recursive with an engine-folder name set so a
  child's exe is attributed to the game it belongs to; single-game
  containers resolve to the child instead of rowing themselves.
  Verified against the reported scenario: `Games` and `Steam` each
  surface exactly their games with binary-metadata titles and no self
  rows. TDD: nested container, deep nesting, game-root engine folders.
- Docs: README scanning section + scope.md v0.7 known limits updated for
  the new semantics.

## 2026-07-21 — v0.7.2: platform dirs, magic candidacy, metadata-first titles

Second field rejection of the scan work, root-caused against the user's
real `games.json` and filesystem: rows named `Steam` (a Steam *client*
install dir with `steam.exe` at its top) and `steamapps` (a `-rwxr-xr-x`
shader cache `.foz` accepted as its "main exe"), a junk-row zoo from
engine/redist trees (`tools/redmod`, `Engine/.../CRS`,
`_CommonRedist/DotNet/4.8`, `__Installer`, `_Redist`), wrong-level rows
(Witcher 3 at `bin/x64_dx12`, Crysis at `Bin64`, 007 at `Retail`), and
folder-name titles where PE metadata existed (`Dead Space.exe` is 423MB —
the 128MiB whole-file cap silently dropped its title; Unreal's
`BootstrapPackagedGame` placeholder rowed Tempest Rising).

- `f30ed02` v0.7.1 review fix-forward baseline: classify visited set
  (symlink fan-out), unreadable-dir tolerance, cache v3.
- `5a89eee` fix(discovery): unix exe candidacy requires PE/ELF magic
  bytes (exec bit alone proved nothing on permissive mounts); fixtures
  carry MZ magic.
- `20f59a3` feat(pever): `TitleFromFile` — 64KiB header window + bounded
  resource read via ReaderAt, no size cap; rejects
  `BootstrapPackagedGame`.
- `9fa6edf` fix(discovery): engine folders (`engineFolderName`, extended
  with plumbing + Proton/SLR prefix rules) are transparent for BOTH game
  and container kinds in the classifier and scanLevel — support trees
  never row and never force the parent into a container.
- `a50818d` fix(discovery): container children outrank a dir's own exe;
  `steam.exe` + `Steam.dll` platform sentinel; engine-named scan roots
  refused.
- `b8b3f2f` feat(discovery,app): title chain PE metadata → exe stem
  (platform tokens stripped, camel split, echo/generic guards) → folder;
  `manualName` delegates.
- `ef21b04` fix(discovery,pever): separator-insensitive exe ranking
  (`FarCry5.exe` matches "Far Cry 5"); `exe`/`steamworks shared` engine
  names; `UE4Game` placeholder rejected.
- `4b0607f` fix(ui): games cache schema v4 — v1–v3 caches carry rows the
  new scanner rejects; warm boot falls through to a real scan.
- `8063b7f` fix(ui): covers progress ticks every non-scanOnly extra root
  (dedup and failure included) — no more stalled phase.
- Verified against the user's real library (scratch probe, deleted after):
  111 rows in ~12s — no Steam/steamapps/Proton/compatdata/redist rows;
  Witcher 3 / Crysis / 007 / Cyberpunk row at their roots; `Dead Space`
  titled from PE metadata; Tempest Rising and Obduction titled by stem.
- Docs: README status + scanning paragraph, scope.md v0.7 known limits
  rewritten (two stale v0.7 bullets removed), architecture.md covers-tick
  paragraph, this entry.

## 2026-07-21 — v0.7.2 review gate: fix-forward after R1/R5 FAILs

Five-lane gate on b8ef5ab: R2 QA PASS (13/13 adversarial scenarios),
R3 quality PASS, R4 security PASS; R1 goal FAIL (two residual junk-row
paths) and R5 context FAIL (wrong InjectionDir, lost game, stale
evidence). Fix-forward:

- `24ff154` fix(discovery): exe walks prune plumbing subtrees
  (`downloading`, `compatdata`, `shadercache`, `temp`, `music`,
  `sourcemods`, `steamworks*`, `steamvr`) and a Wine prefix's
  `drive_c/windows` / `drive_c/users` — a game-less steamapps with a
  partial download no longer rows as "steamapps", and a bare prefix no
  longer rows via `notepad.exe`. `maxExeDepth` 3 → 4: Prey
  (`Binaries/Danielle/x64-Epic/Release/Prey.exe`) is back in the
  library; Days Gone's CRS stays dead (container via BendGame first).
- `880f700` fix(discovery): separator-insensitive name scoring in
  `ResolveInstallDir` — Far Cry 5's InjectionDir resolves to `bin/`
  again (the 60MB .NET installer previously won the tie; an Install
  click would have targeted a redist folder).
- `fe177b1` hardening: `resolveGameExe` falls back to FindMainExe when
  the parent is engine-named; UTF-8-safe stem splitting; TitleFromFile
  negative-size guard; non-blocking magic-sniff open (FIFO swap race).
- Real-tree probe re-captured at HEAD (`/tmp/v072-real-rows.log`): 109
  rows in 13s — PREY restored, Far Cry 5 InjectionDir = `bin`, zero
  plumbing/junk rows; the earlier 111-row artifact predated the ranking
  fix and has been replaced. `HELLDIVERS™ 2` shows correctly in
  production (the exe's own ProductName string is double-encoded by the
  vendor; the Steam manifest name wins the dedup).
- Remaining accepted items: codename PE titles (`GoWR`, `b1`, `TOI`,
  `SilentHill`, `witness64 d3d11`, `Anvil`, `Cardinal`, `STASIS2`,
  `NobodyWantsToDie`) are metadata-first per the user's contract —
  presented to the user as an explicit decision; covers phase can clear
  before its final 100% frame renders (cosmetic).

## 2026-07-21 — v0.7.2 gate round 2: R1/R5 PASS, mojibake guard

R1 re-review found three same-mechanism leaks (walk descended
engine-named dirs that are never game material): Proton-only `common`
rowed "Wine", `workshop/content` rowed next to real games, and the
depth-4 bump re-admitted the CRS class. `dfcf0c3` walk-prunes
Proton/SteamLinuxRuntime (shared `platformToolName`), `workshop`, and
`ThirdParty`/`third_party`, and unifies the skip-token lists
(`unrealcefsubprocess`, `prerequisites`). R1 then passed with a
real-tree worktree diff (108 → 109 rows, zero lost, exactly +Prey).
R5 passed on real data: FC5 InjectionDir = `bin`, PREY restored, all
externals reconcile — plus a newly detected Cyberpunk 2077 external
install (0.7.9) every prior scan missed. Residual landed: `usableTitle`
rejects vendor-baked mojibake (U+FFFD / double-encoded `ï¿½`), so
Helldivers 2's damaged ProductName falls through to its clean
FileDescription on the manual path. Row counts: the definitive HEAD
artifact is 109 rows (the historical 111 figure predated the ranking
fix; the QA lane's independent probe reports 110 including
manifest-sourced rows outside ScanRecursive's scope).

## 2026-07-21 — v0.7.2: user title decision — junk-list + dedupe

User decision on codename PE titles (GoWR, b1, TOI, SilentHill, …):
keep metadata-first, reject objective junk, disambiguate duplicates.
- `usableTitle`/`junkTitle` now rejects vendor-junk strings
  ("Electronic Arts System Information", "Shockwave Flash",
  "Elevate Application"/"Elevate", "Easy MFC Application",
  "Macromedia Flash Player*") so repacked/tool exes fall through to
  FileDescription → stem → folder: Generals Zero Hour, Samorost2, and
  RSI Launcher read correctly on the real tree.
- Duplicate titles get a folder suffix at scan assembly
  (`disambiguateTitles`): the two "TOI" games are now
  "TOI (Tails of Iron)" and "TOI (Tails of Iron Bright Fir Forest)";
  when the folder IS the title, the full install dir disambiguates.
- "elevate" joins the generic stem set (the RSI updater exe no longer
  donates its name).
- Verified on the real library (109 rows): Helldivers recursive title
  is the clean "HELLDIVERS 2" (mojibake guard), Generals/Samorost2/RSI
  correct, codenames (genuine metadata) untouched per the user's call.

## 2026-07-21 — v0.8: priority-ordered game identification

The user supplied a production identification spec (hard IDs → store
metadata → engine metadata → canonical DB fuzzy → folder last +
override) after the v0.7.2 title work; scope confirmed interactively
(Phase A offline + Phase B keyless-online, canonical titles supersede
codename PE metadata; IGDB/SGDB deferred as user-cred tiers).

- `8da019f` schema: domain.TitleSource (9-value wire contract),
  Game.SteamAppID, settings.title_overrides, GameRow.TitleSource,
  games cache v5.
- `a835826` internal/gid: offline detection — steam_appid.txt (≤2
  levels, 480 rejected), goggame on all platforms, .egstore with
  InstallLocation guard, Unity app.info; GOG/Epic parsers moved from
  discovery (aliases keep its API).
- `b2032ab` gid fuzzy matcher: NFD-diacritic normalization, edition/
  platform/version noise stripping, strict Jaccard (no subset credit),
  ≤4-char exact-only guard, ≥90 / ≥75+corroboration thresholds.
- `7ed1c01` steam keyless store endpoints: appdetails (appid→name+
  developer), storesearch (term→items with platform flags); shared
  pacing/cooldown/30d caches, negatives cached.
- `80acf40` ChainResolver at row creation (override > in-dir metadata >
  PE > stem > folder; appid always recorded), threaded through
  ScanRecursive/ScanAll/ScanAllLibraries/ManualEntry; Steam rows record
  their appid directly.
- `604d9e9` enrich-phase identifyRow: appid→canonical upgrade,
  fuzzy resolution with developer corroboration
  (pever.CompanyFromFile), ProtonDB reuses resolved appids.
- Feasibility notes (live-probed): keyless bulk GetAppList is dead
  (404), so no offline canonical corpus; appdetails ~200 req/5min; the
  user's Heroic service list was evaluated — only Steam endpoints are
  keyless and title-relevant today.

## 2026-07-21 — v0.8.1: gate fix-forward + PCGamingWiki secondary source

Five-lane gate on v0.8: R2 QA PASS (15/15 probes), R3 quality PASS, R4
security PASS; R1 FAIL (override gaps on AddDirectory + CLI scan,
precedence inversion for metadata-sourced rows with appids), R5 PASS
with honest UX concerns (convergence math, stuck rows, fuzzy edition
traps). Fix-forward `7ce3fd8`:

- TitleOverrides now apply on the AddDirectory path and the CLI scan
  (discovery.CanonicalPath exported for key matching) — "manual
  override always wins" holds on every path.
- Precedence corrected: a known Steam appid takes its canonical upgrade
  before the in-dir metadata switch (hard ID > engine metadata, per the
  user's spec); hostile non-numeric appids from hand-edited caches are
  refused before becoming URLs/cache filenames.
- findSteamAppID tries the next candidate on a rejected parse (a root
  "480" placeholder no longer masks a real steam_settings id); all
  identification reads bounded (4KiB/1MiB/8-file caps).
- Lookup queue prioritizes appid-bearing tail rows — the cheap
  canonical upgrades (GoWR-class) land in scan 1 instead of starving
  behind ~100 fuzzy candidates (budget stays 8; failures cache and the
  queue flows forward).
- DLC store pages resolve when no base app matches (The Talos Principle
  2: Road to Elysium); digit-boundary matching (Samorost2 ↔
  Samorost 2); duplicate-title suffixes use the parent dir
  ("Red Dead Redemption 2 (Games)" not a raw path).
- internal/pcgw: PCGamingWiki as the secondary canonical source when
  Steam finds nothing (the only viable service from the user's Heroic
  list — keyless, cargoquery HOLDS + opensearch, 30 req/min pacing,
  30d caches with negatives; TheGamesDB/UMU/HLTB/Deck/AppleGamingWiki
  rejected per the feasibility review).

## 2026-07-21 — v0.8.1 gate: R1 PASS after fix-forward

R1 re-review: PASS (HIGH) — all three blocking items verified fixed
(overrides on AddDirectory + CLI, appid-beats-metadata precedence),
exact-match corroboration deliberately rejected as a false-negative
factory per the user's spec. Two WARNs landed same-day: storesearch
caches written before StoreItem.Type existed now serve their items as
apps (v0.8.0 warm caches kept working), and the README privacy note
lists pcgamingwiki.com.

## 2026-07-21 — v0.8.2: bootstrap-shim exe picking + one-scan convergence

User report: "Black Myth Wukong" titled `b1`, and "some binaries are
shims for actual binaries in deeper directories". Both classes confirmed
on the real library:

- UE bootstrap stubs (hundreds of KB, no PE title) won exe picking via
  folder-name match while the real `-Win64-Shipping.exe` (with the real
  title) sat deeper. `findMainExe` demotes small root exes for bigger
  prefix-matched shipping binaries (fbb82e2): Layers of Fear, Tempest
  Rising, Oblivion Remastered, Empire of the Ants, Obduction, Dispatch
  now pick the real binary. Black Myth itself already picked its real
  695MB exe — whose PE ProductName literally is "b1" (vendor codename).
- The fuzzy fix for codenames was real but unreachable in practice at 8
  live rows/scan (~10+ rescans for a tail row). `lookupBudget` is now a
  128-row sanity cap with pacing + 429 cooldown as the rate control; the
  whole library converges in one scan. Verified live: Black Myth →
  "Black Myth: Wukong" in scan 1.

## 2026-07-21 — v0.8.3: cover binding fixes

User report: 11 mismatched covers (CoH3→"Anvil Empires", AC
Shadows→"Shadows on the Vatican", Black Myth→"B1", …) and missing
covers (007 First Light, Oblivion, ControlLauncher). Audit of the live
cache confirmed every binding. Fixes in `2ce79a1`: toRow prefers the
resolved SteamAppID (direct CDN, no search); store search candidates
are scored (gid.Score, ≥90 to bind) so first-hit mistakes can't bind;
art falls back to library_hero.jpg before the placeholder (007,
Oblivion, Aphelion have banners, no portrait art); refreshCovers
rebinds after the identification phase each scan; 7-day miss markers
for artless appids; "reloaded" edition token for store rebrands
(Dying Light 2). Real-library re-audit: all 14 previously bad covers
now correct.

## 2026-07-21 — v0.8.4: portrait art everywhere, emulator tooling

User report: landscape banners instead of posters (007, Aphelion,
Oblivion), artless games (Alan Wake 2, From Dust, Jazz Jackrabbit,
STASIS2, Deadpool), and Zelda discovered as "cemu". Fixes in `673c930`:

- PCGamingWiki box art (portrait) in the cover chain between Steam's
  600x900 and the landscape hero — posters for Steam-artless games,
  and the only art source for non-Steam games. The wiki's thumbnail
  host 403s non-descriptive UAs (and Go's default): covers now sends a
  proper User-Agent on every request.
- Exact matching maps roman-numeral tokens (i/v/x only): Alan Wake
  II ↔ Alan Wake 2.
- In-dir metadata rows without an appid now go fuzzy instead of
  freezing: "STASIS2" → "STASIS: BONE TOTEM" (title + cover).
- Emulator dirs (cemu/yuzu/ryujinx/dolphin/pcsx2/rpcs3/xenia/citra/
  retroarch) are walkable tooling: console dumps row at their root with
  the emulator exe; emulator names are rejected as titles ("Cemu",
  "Wii U emulator") — Zelda is "The Legend of Zelda - Breath of the
  Wild" again.
- Real-library verified: all listed cases fixed.

## 2026-07-21 — v0.8.5: GUI polish (7 fixes)

- Text fields: single shared `themedInput` at `fieldH=28` everywhere;
  real caret (2×16 `focusBorder` bar) — the invisible caret's true
  cause was a `HasFocus()` scope bug (evaluated inside the inner row,
  always false), now captured at the focused box.
- `/` focuses the search field from anywhere.
- Card buttons pin to the bottom (`Filler(1)`); uniform card heights
  with/without pill rows.
- Wayland CSD titlebar disabled (vendored `csdEnabled = false`) — OS
  decorations win; scroll ×2/×3 in vendored `ScrollOnInput`. Both carry
  patch markers + a guard test + docs/vendor-patches.md entries.
- List rows keep a 20×30 cover thumbnail.
- Sidebar fills window height; nav icons vertically centered, Exit at
  bottom.
- Verified headlessly (slash-focus, bottom-Y alignment, sidebar height)
  and by rendered-PNG inspection of all seven items.

## 2026-07-21 — v0.8.6: cover-delete crash + text fields v2

- Crash: `shirei.ReadFileContent` nil-derefs on a missing file
  (`os.Stat` error ignored). Deleting the cover cache panicked startup
  with cached CoverPaths. coverArt stat-guards → placeholder tile
  (pinned by TestCoverArtMissingFileDoesNotPanic).
- Text fields v2: fieldH 24 / Pad2(2,sp12); REAL cursor
  (arrows/Home/End, 500ms blink, flush against text via Gap(0)); REAL
  selection (Shift+arrows/Home/End, Ctrl+A, Backspace/type-replace,
  blue highlight band, Ctrl+C/X/V via RequestTextCopy/RequestPaste);
  per-field keyed editState + themedInputState test seam. Verified by
  behavior tests and rendered-pixel inspection.

## 2026-07-22 — v0.8.7: text fields v3 — centering + mouse editing

- Vertical centering: the field box was a column (text stacked at the
  top); it is now a Row with CrossMid, and the caret matches the shaped
  line height exactly (FixSize(2,13) — was 16, which grew the row on
  focus and re-centered the text by 1.5px: the click-jitter). Row
  geometry pinned identical across hint/caret/text states.
- Mouse editing (editMouse): click positions the caret at the clicked
  glyph, shift+click extends, double-click selects the word,
  triple-click selects all, click-drag selects the range. Hit-testing
  shapes through shirei's own ShapeText at the field's exact style and
  walks glyph advances with a midpoint rule (rune-index clusters,
  multibyte-verified); the text-flow container records its screen rect
  per frame (st.textRect) and spans the field width so clicks past the
  text clamp to the end.
- Pixel-verified: drag band x-run [35..63] == shaped prefix widths
  [35.4..63.5]; glyph ink top-y identical (14) in plain and selected
  states.
- Incident: an external process ran `go mod tidy` + `go mod vendor`
  mid-session, bumping pins and wiping the vendored shirei patches;
  TestVendorCSDPatchPresent caught it and the tree was restored
  (git checkout + git clean). The guard works — keep it.

## 2026-07-22 — v0.9.0: list focus nav, upgrade action, Linux-only ProtonDB, terminal-editor INI, README revamp

- ProtonDB is Linux-only: enrichment skips the ProtonDB summary call
  off-Linux (injectable GOOS seam on ui.Deps, "" → runtime.GOOS, the
  launch.New idiom), and games.json caches written on Linux are stripped
  of tiers when loaded off-Linux (strip-at-load, no schema bump). The
  resolved Steam appid is still kept for identification on every
  platform. Display sites already no-op on an empty tier.
- README is user-first: the release-history dump is gone (docs/log.md
  owns history), features are presented for users, and the technical
  content (architecture, stack, conventions, release process) moved to
  the new README.dev.md, linked from the README's Development section.
- The list view is keyboard-navigable with a real focus model: Tab cycles
  focus into the list (visible focus ring), Up/Down move the selection one
  row and scroll it into view, Enter toggles the detail panel, and Tab
  exits to the next focusable element. When the list is not focused the
  global arrow/Enter fallback keeps working (grid mode unchanged). Shared
  move/toggle logic factored onto the model so the focused path and the
  fallback cannot drift apart.
- Games running an outdated OptiScaler get an Upgrade action: the session
  resolves the preferences' default version to a concrete tag once per
  setting (cached, never per row), compares it against the installed
  version with a new dependency-free semver-ish comparator
  (internal/version: leading-v normalized, numeric triple, pre-release
  older than release), and marks eligible rows (committed or external).
  The quick action on cards and the detail panel reads "Upgrade to X" for
  eligible games. Upgrading a managed install chains uninstall then
  install through the existing backup/rollback paths (a crash between
  steps leaves the game without OptiScaler — the window is deliberate,
  rollback cleans any partial state and the row shows the failure);
  upgrading an external install adopts with the usual external backup.
  QuickInstall dispatches to Upgrade for eligible rows, so the committed
  toggle (uninstall) can never fire by mistake on an outdated game.
- "Open OptiScaler.ini" on Linux now opens the file in the user's
  terminal editor inside a terminal emulator: the editor comes from
  $EDITOR (verbatim), falling back to micro, then nano, then vi; the
  terminal comes from $TERMINAL (basename-matched argv conventions:
  foot/kitty positional, konsole/alacritty/xterm -e, gnome-terminal --,
  unknown terminals get a best-effort -e) falling back through
  foot → konsole → gnome-terminal → kitty → alacritty → xterm, spawned
  detached (new internal/termopen package, modeled on internal/launch).
  Windows (rundll32) and macOS (open) behavior is unchanged, and the
  session's openExternal seam is preserved.
- Hardening from the binding review: games.json no longer resurrects
  stale upgrade offers on warm boot (eligibility is stripped at cache
  load on every platform and recomputed from a fresh memo on the next
  scan), and an upgrade whose install leg loses the op-slot race now
  routes through the same rollback cleanup as other install failures
  (the game is never silently left without OptiScaler).
- The card consistency fix (slimmer padding, measured button height,
  first-button click seam) was REVERTED on request (74b0b39): the zero
  vertical padding made card top margins too tight. Cards are back to
  10px padding on all sides; the two card tests
  (TestCardButtonClick_FiresActionNotSelect,
  TestGUICardButtonsBottomAligned) are red again with their pre-fix
  signature, owned by the in-flight card rework.

## 2026-07-22 — v0.9.1: TUI opens the ini in its own editor

- The TUI's "open INI" (`o` on the detail screen) no longer spawns a
  terminal emulator: it suspends the TUI and runs the user's terminal
  editor in place via tea.ExecProcess — the charmbracelet mechanism for
  external TUIs — resuming when the editor exits (an external process
  cannot render inside a subwindow, so the editor takes over the
  terminal as a modal). The editor chain is shared with the GUI via the
  newly exported termopen.Editor ($EDITOR → micro → nano → vi), and the
  ini path resolver is Session.INIPath (pure, no side effects).
  Session.Toast lets the frontend report editor failures. The GUI keeps
  the termopen spawn (no terminal to suspend there).

## 2026-07-22 — v0.9.2: upgrade-offer gaps closed (oracle review)

- SetDefaultVersion now clears every live upgrade offer the moment the
  default changes (previously rows kept showing "Upgrade to <old target>"
  until the next scan while a click would install the NEW default — a
  caption/action lie; a same-value write is a no-op and keeps offers).
- Offline scans no longer suppress upgrade offers for a PINNED default:
  an exact tag needs no resolution, so runScan memoizes it directly with
  online lookups off (never touching the resolver; "latest" still yields
  no offer offline). The offline memo is provisional — a zero timestamp
  lets refreshResolvedDefault re-resolve for real once lookups are back
  on, and installs keep their stale-cache consent semantics.
- The TUI advertises upgrades: the games list renders an accent
  "↑<target>" badge on eligible rows and the detail screen's i action
  reads "upgrade to <target>" (overriding the external "adopt" caption,
  matching the GUI's quickLabel). Tests drive the real session flow
  (install at an older tag, retarget, rescan) because the games cache
  strips offers on load by design.

## 2026-07-22 — v0.9.3: warm boot recomputes upgrade offers

- Session.Start's warm-cache path now re-resolves the default version and
  recomputes upgrade offers on the cached rows (async, EvOffersRefreshed
  poke): the games cache strips offers on load as a stale-offer defense,
  so before this fix a restarted app showed NO upgrade offers until a
  manual rescan — reproducible on the real library (UNCHARTED committed
  at 0.7.9 with default "latest"=v0.9.4 showed nothing). Offline behavior
  matches scans: pinned defaults offer, "latest" stays silent.
- The TUI upgrade badge now LEADS the badges cell: appended last it was
  the first badge the fixed-width truncation ate on tech-heavy rows
  (real-row evidence: "[DLSS] [DLSS-FG]…" with the offer invisible), so
  "[↑v0.9.4]" now renders ahead of the tech badges.

## 2026-07-22 — v0.10.0-wip: per-game version management (wave 1)

- internal/app.CachedVersions enumerates downloaded bundle versions:
  subdirs of <cache>/optiscaler containing at least one Optiscaler_*.7z
  regular file (filters .download-* temps and strays), sorted
  semver-descending via version.Compare; missing cache yields nil, no
  error. Feeds the per-game version dropdown's "available in cache" list.

- Session.SwitchVersion(gameDir, version) switches an installed game to a
  chosen OptiScaler version while preserving the game's OptiScaler.ini:
  committed rows chain uninstall→install at the chosen tag, external rows
  adopt-install; the ini is captured (bytes+mode) before the switch and
  written back after the install leg — and removed BEFORE the uninstall
  leg, because the installer refuses to uninstall foreign-modified files
  (restored immediately if the leg refuses or is busy). Install path
  gained a version parameter (runInstallVersion; "" = configured default,
  byte-identical behavior for existing callers) and Confirmation gained a
  Version field so an install paused at the EAC/stale-cache gate resumes
  at the SAME tag. Consent gates (EAC, stale-cache prompt, rollback on
  failure-after-uninstall) behave exactly as before; a failed ini
  write-back surfaces as a warning toast and the install stands. Known
  edge: a switch resumed from a consent pause gets curated ini defaults
  (same gap as doUpgrade's pause). Same-version selection is a silent
  no-op. Old upgrade-offer machinery untouched (retired in a later wave).

- Session.Versions(gameDir) composes the per-game selectable-version
  list for the dropdown/cycler: unique(installed ∪ CachedVersions ∪
  preference), sorted semver-descending. A "latest" preference
  contributes only the memoized resolved tag (never the literal
  "latest"; nothing when unresolved); a pinned preference is always
  contributed verbatim, even offline and uncached. The method never
  touches the network or resolver — it reads the memo and the
  filesystem; resolution remains a scan/warm-boot concern. Dedupe is
  exact-string (installed "0.7.9" may coexist with cached "v0.9.4");
  unknown game dirs yield cached ∪ preference; empty everywhere yields
  an empty non-nil slice.

- The upgrade-offer model is retired in favor of explicit per-game
  version management: GameRow.UpgradeAvailable/UpgradeTarget,
  upgradeOffer, Session.Upgrade/doUpgrade, QuickInstall's offer dispatch
  (a quick click on an upgrade-eligible committed game now UNINSTALLS —
  the caption and the action agree again), the cache-load offer strip,
  and EvOffersRefreshed are gone. The resolved-default machinery stays
  (the version dropdown reads the memo): the warm boot now performs a
  resolve-only warmBootResolveDefault. GUI quickLabel and the TUI
  badge/hint lost their upgrade branches (plain Uninstall/install
  captions); their redesign lands in the next waves.

- GUI: the OptiScaler version pill is now a per-game version dropdown on
  grid cards and the detail panel (installed rows only). Composed locally
  per the modal() precedent — upstream MenuButtonExt is theme-locked
  light (_menuBG) — from a pill-geometry focusable trigger (13px tall,
  cardContentH untouched) plus a Popup surface (bgPanel/border/
  elevateOverlay, below-trigger window-clamped) listing
  Session.Versions: current version ticked, hover accent, Esc and
  click-outside dismissal, one dropdown open at a time. The version list
  is computed when the dropdown OPENS and on frames while open (closed
  triggers do no cache I/O, so idle grids stay free).
  Selecting a different version dispatches SwitchVersion; re-selecting
  the current one never dispatches. The card press-guard gained
  overDropdown so the card body can't steal the trigger's activation
  (same fix class as overButtons).

- TUI: per-game version cycling with v on the games and detail screens.
  The first v stages a switch on installed rows (computed from
  Session.Versions at the keypress, never per frame), the version cell
  shows the staged candidate (truncated before styling, width-pinned),
  further v advances with wraparound, Enter confirms (dispatching
  SwitchVersion only when the candidate differs from the installed
  version — wrapping back to current dispatches nothing), Esc cancels,
  and any other key drops the stage and falls through (cursor moves,
  screen switches, rescans all reset for free). The detail OptiScaler
  line shows "cur → candidate" while staging and the Actions block,
  footers, and help screen document the binding; not-installed rows
  show no hint and v is a no-op.

- Reviewer-driven hardening of the version switch: EAC consent is now
  PRE-FLIGHTED (new ConfirmVersionSwitch kind carrying the chosen tag) —
  nothing destructive (ini capture/removal, uninstall) runs before the
  user answers, so declining leaves the game and its OptiScaler.ini
  untouched, and accepting re-enters the full switch chain with consent
  (never a bare install that would drop curated defaults). The ini
  capture is crash-durable (bytes+mode staged to
  <data root>/ini-switch-backup-<sha>.tmp before removal, deleted after
  the write-back; a failed staging aborts the switch), and rollback /
  cancelled-install paths write the ini back as an orphan in the clean
  game dir. Dropdown keyboard path pinned by characterization tests
  (Tab focus ring, Enter/Space toggle, Esc closes the dropdown before
  the detail panel).

## 2026-07-22 — v0.10.x: version-dropdown dedupe, Shift+Tab, click focus, list nav

- Session.Versions dedupes SEMANTICALLY (version.Compare == 0) instead of
  exact-string: an installed "0.9.4" and a cached/resolved "v0.9.4" no
  longer appear twice; on collision the installed form survives so the
  dropdown tick and same-version no-op keep working. The dropdown's tick
  and dispatch guard compare semantically too. Pre-release tags stay
  distinct ("v0.9.4-test" never merges with "0.9.4").

- Shift+Tab now reverse-cycles focus on Wayland: the backend's xkb
  KeyGetOneSym returns ISO_Left_Tab (0xfe20) with Shift held, which
  mapKeysym didn't know, so the keypress vanished. One-line vendored
  patch maps xkISOLeftTab to KeyTab (marked PATCHED by optiscaler-manager
  (v0.9), guarded by the extended csd_test marker check, registered in
  docs/vendor-patches.md). X11/Win32/Cocoa needed nothing; the toolkit's
  reverse cycling was already correct.

- List view navigation: clicking a row now also moves the keyboard
  cursor onto it (selIdx) and focuses the list — the focus grab is
  re-asserted on the next frame via a listFocusPending flag because
  shirei identities are path-scoped and the opening detail panel
  re-nests the wrapper, orphaning a focus set mid-click. The
  session-selected row renders the selBg band (previously used only for
  text-selection highlight)
  independently of the cursor row's accent border; selected wins over
  hover so the open game stays visible while the pointer roams. Enter's
  detail toggle is unchanged.

- Clicking a focusable control now moves keyboard focus to it:
  focusableButton, focusableToggle, and the version-dropdown trigger all
  call FocusOnClick (previously only the search input and the list had
  it), so click-then-type/Enter flows work without a manual Tab first;
  clicking empty space blurs the focused control. Activations are
  untouched (FocusOnClick only moves focus).

## 2026-07-22 — toolbar + grid keyboard focus

- The toolbar Sort button is now a local focusable dropdown (replacing
  the upstream MenuButtonExt, which was keyboard-unreachable and painted
  the theme-locked light _menuBG popup): a Tab stop with a focus ring,
  click-to-focus, Enter/Space toggling a dark Popup with the same two
  sort items, Esc/click-outside dismissal, disabled when the library is
  empty. A pick calls setSort and keeps focus on the trigger (explicit
  FocusImmediateOn — the pick click blurs it on the down-frame).

- The grid/list view toggle is a single Tab stop with a focus ring:
  clicking a segment now also focuses the wrapper (FocusOnClick), and
  Enter/Space toggles the view with the key consumed (inert when the
  library is empty, matching the segments' disabled guard). Focus is
  retained across toggles — the toolbar's identity is stable above the
  conditional detail-panel row.

- Grid keyboard focus, mirroring the list model: the grid is one Tab
  stop with a focus ring; while focused, arrows move the card cursor and
  Enter opens the detail pane for the cursor card (consumed, no double
  fire from the global fallback). Clicking a card moves the cursor onto
  it and grabs grid focus, retained through the detail panel opening via
  a deferred gridFocusPending re-assert (the panel re-nests the grid and
  shirei identities are path-scoped). The cursor card shows a
  focusBorder ring distinct from the hover accent; grid-mode arrows now
  scroll the cursor card into view (previously list-only). Card layout
  and geometry untouched.

- Grid focus reworked per-user: the grid CONTAINER is no longer
  focusable — the cards themselves are. Clicking a card focuses it
  exclusively (inner controls still win their own clicks), Tab cycles
  card → its dropdown → its buttons → next card, Enter on a focused card
  opens the detail pane, and arrows on a focused card move cursor and
  focus together (immediate for backward moves, deferred cardFocusPending
  for forward/off-screen and for surviving the detail panel's re-nest).
  One ring only: the focused card, or the selIdx cursor when nothing is
  focused. Ring clipping fixed: shirei draws borders straddling the rect
  edge and the chunk row's Clip sat flush with zero vertical padding,
  scissoring the top/bottom half-strokes — 1px of vertical row padding
  (spacing only, card geometry untouched).

## 2026-07-22 — dropdown arrow nav + focus exclusivity

- Both dropdowns are keyboard navigable: with the menu open and the
  trigger focused, Up/Down move a highlight (wrapping), Enter activates
  the highlighted item (reusing the exact click-pick path), and the
  highlight initializes on the current item at each open (sort mode /
  ticked version) and follows the mouse so input modes never fight.
  Applies to the sort menu and the per-game version dropdown alike.
