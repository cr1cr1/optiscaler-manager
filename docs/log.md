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

## 2026-07-20 — W5-T7: bubbletea TUI frontend on ui.Session

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
