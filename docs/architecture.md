---
type: explanation
---

# Architecture

## Shape

CLI-first Go app with a GUI default command. The existing kong shell
(`cmd/root.go`: `RootFlags`, `DefaultEnvars("OM")`, `ExitError` codes —
2 usage / 1 runtime, zerolog setup) is preserved; the GUI wraps it, never
replaces it. No-args launches the GUI via kong `default:"withargs"`.

## Package map

```
main.go                     thin wrapper → cmd.Run
docs_okf_test.go            OKF compliance gate (package main)
cmd/
  root.go version.go        existing shell (RootFlags; version subcommand)
  gui.go                    GuiCmd `cmd:"" default:"withargs"`; --audit-grid
  tui.go                    TuiCmd; second frontend on the same session
  session.go                newSession: shared session construction (gui,
                            tui, and the five one-shot ops commands)
  scan.go                   headless game listing (store + versions columns)
  install.go                headless install/uninstall/rollback <path>
  opwait.go                 one-shot CLI helpers: waitForOp(Kinds) event
                            waiter, answerGate consent prompt, awaitRow
                            boot settle, opTimeout
  switch.go                 SwitchCmd: version switch incl. `latest`
  dlss.go                   DLSSUpdateCmd + DLSSRestoreCmd (default newest)
  launch.go                 LaunchCmd: fire-and-forget launch request
  hook.go                   HookCmd: --enable|--disable (synchronous)
  deps.go                   shared Deps wiring (GH/DLSS/Launcher/Covers
                            test seams) + interrupted-manifest startup
                            warning (gated off gui/tui/version)
internal/
  domain/     Game (Store enum, AppName, ExePath, CompatPrefix), Release,
              Component, Kind, Manifest, entries; Status
              state machine: 4 persisted (in_progress/committed/failed/
              rolled_back) + 1 derived (external, scan-time only)
  store/      manifest + backup persistence (external root)
  discovery/  multi-store scan. OS-agnostic parsers (Steam VDF, Epic .item,
              GOG goggame info, recursive roots, plist) test on every GOOS;
              build-tagged OS probes (Steam roots, Epic manifest dirs, GOG
              registry via a registry-reader seam, macOS /Applications .app,
              linux Proton compat prefix) compile per-GOOS. ScanAll merges
              Steam → Epic → GOG → apps → manual, deduped by canonical
              InstallDir; install-dir resolution; ClassifyGameDir sorts a
              directory into game / container / empty (bounded walks, no PE
              parsing — see the v0.7 section)
  classify/   upscaler kind+DLL detection (Dir, DirFiles)
  pever/      hostile-input PE version-resource parser (no cgo): FileVersion,
              MarketingName (vendored DLSS/FSR/XeSS version→name maps),
              OptiScalerVersion (manifest → log → ini evidence chain),
              DetectOptiScaler (external-install probe: injection-name
              candidates matched by PE version-info identity, bounded reads)
  gh/         GitHub releases: glob asset match, cooldown cache
  dlss/       NVIDIA DLSS runtime updater (opt-in, on user action):
              resolves NVIDIA/DLSS main to an immutable commit, downloads
              the three lib/Windows_x86_64/rel DLLs at that commit into a
              commit-keyed download cache under cacheDir (OptiScaler
              bundle-cache pattern), and keeps transactional snapshot
              backups (hash-verified) under the state root for restore
  archive/    7z extraction with hostile-input defenses (sevenzip)
  installer/  transaction core: stage → validate → backup → copy → manifest;
              rollback; uninstall; EAC check; ctx cancel at phase boundaries
              (cleanup under context.WithoutCancel)
  profile/    curated OptiScaler.ini writer
  covers/     cover art: Steam CDN by appid → PCGW → hero image → scored
              title search (best candidate, PC tie-break) → placeholder;
              disk cache plus a 7-day `.miss` negative marker that skips
              only the CDN retry, never the title search
  steam/      title → appid lookup (steamcommunity.com SearchApps; 30d TTL
              disk cache, no auth)
  protondb/   appid → compatibility tier (protondb.com summaries API; 7d
              TTL disk cache, 429 cooldown)
  pcgw/       PCGamingWiki secondary title source (keyless MediaWiki API:
              opensearch + Cargo reverse lookup; 30 req/min pacing,
              429/5xx cooldown, 30d disk cache with negatives)
  jsoncache/  shared JSON state-file helpers for the API client caches
              (generic Read/Write, 429/5xx cooldown markers); used by
              steam/, protondb/, and the steam store caches
  settings/   persisted preferences (settings.json in the data root):
               default version, launch template, extra dirs, online
               lookups, card size (CardSize type with OrDefault())
  version/    OptiScaler version-string ordering for upgrade eligibility
              (leading-v normalized, numeric segments, pre-release older
              than release; deliberately not full semver)
   pickdir/    OS directory dialog (zenity→kdialog on Linux,
               PowerShell FolderBrowserDialog on Windows, osascript on macOS)
   umu/        umu-launcher integration (Linux only). Detect parses
               `umu-run --version`; FindRunners scans Steam
               compatibilitytools.d, Bottles runners/, and umu
               compatibilitytools for Proton builds; Launch wraps the
               exec with the GAMEID/WINEPREFIX/PROTONPATH/STORE env and
               scans stderr for umu's exit-0-on-fatal-error quirk;
               PrefixFor derives a per-game prefix under
               <state-root>/umu-prefixes/<sha1[:12]>; IsWindowsBinary
               detects PE MZ binaries for eligibility.
   launch/     per-store per-OS command table (pure Command fn) + detached
               spawn (build-tagged spawners; Start + Process.Release, never
               Wait). Steam steam://rungameid (Proton is Steam's business),
               Epic launcher URL, GOG direct exe, manual user template split
               without a shell. Never `proton run`; umu-run invocations
               live in internal/umu and are wired via Deps.UmuLauncher in
               Session.doLaunch (manual-store Windows binaries on Linux)
  termopen/   open a text file in the user's terminal editor, detached
               (Linux): $EDITOR verbatim, $TERMINAL's basename picks the
               run-a-command convention, else the foot→konsole→
               gnome-terminal→kitty→alacritty→xterm chain; no shell
  app/        shared orchestration: ScanAllLibraries (version
              enrichment via classify+pever on managed installs;
              probeInstallState adds the on-disk external/disabled probe
              for unmanaged rows), Install, Uninstall, Rollback,
              UpdateDLSS/RestoreDLSS/DLSSSnapshots (NVIDIA runtime set),
              ManualEntry (+WithResolver), CachedVersions, versioned
              bundle cache, ops.go (Op, RunOps: errgroup, first error
              cancels siblings)
  ui/         frontend-agnostic Session: state, commands, events, consent,
              per-game CancelOp, sort mode; session.go keeps the types and
              shared state helpers while the themed methods live in
              scan.go (pipeline + warm-boot reconcile), dirs.go,
              install.go, launch.go, settings.go, browse.go, lookup.go +
              identify.go (online identification), switch.go + upgrade.go
              + versions.go (version switching, default memo), disable.go
              (hook toggle), dlss.go (NVIDIA runtime update/restore ops +
              confirmation), probe.go (selection-time re-probe), rows.go
              (GameRow + display helpers); cache.go is the games.json
              library cache (schema-versioned, atomic) behind
              Session.Start
  gui/        shirei binding over ui.Session (ALL shirei imports live here);
              theme tokens, arrow-key grid nav, right-docked detail panel
  tui/        bubbletea binding over ui.Session (renders snapshots, forwards
              keys; no business logic beyond the in-process OptiScaler.ini
              editor for `o`); multi-screen styled layout on
              bubbles spinner/textinput/viewport + lipgloss
```

`internal/installer` is the deep module for file transactions. `internal/app`
sequences domain packages into workflows both frontends share. `internal/ui`
adds interactive session semantics (async commands, event stream, consent
gates, toasts) with zero display-toolkit imports; `internal/gui` and
`internal/tui` render its snapshot and forward commands.

## Cross-platform shape (v0.3)

The tree cross-compiles to windows with CGO off (`GOOS=windows go vet ./...`
is a CI gate). Darwin is package-scoped only: shirei's cocoa backend needs an
Apple SDK, so CI gates darwin with vet + `go test -c` on the non-GUI packages
(`internal/discovery`, `internal/launch`). Cross-compiled test binaries are
never executed (no wine/darling on runners); the gate is compile-only (W3
decision). Release artifacts: linux/amd64, windows/amd64 (goreleaser, CGO
off). macOS artifacts wait on a macos-latest release runner — see
`.goreleaser.yml` for the full diagnosis.

## Cancellation model (v0.3)

Install/uninstall/rollback carry a context; checks sit at every phase
boundary. A cancelled op marks the manifest `failed` (cause recorded) and
rolls back under `context.WithoutCancel` — cleanup belongs to the same atomic
op, so it must outlive the dead op context while keeping the caller's values.
`internal/app/ops.go` runs batches as an errgroup: first error cancels
siblings. `ui.Session.CancelOp(gameDir)` cancels a single in-flight op and
settles the row to its pre-op status with one "Cancelled" event. Invariant #6
in `docs/safety.md`.

## Startup flow: games cache (v0.4)

Both frontends boot through `Session.Start(ctx)`. It reads `games.json`
from the data root (`internal/ui/cache.go`: schema-versioned envelope,
atomic temp-write + rename, best-effort writes that log and never fail the
caller). A warm cache hydrates the rows synchronously; each row's status is
then reconciled from the store's manifests (keyed by canonical install dir,
falling back to game root) so installs that settled while the manager was
not running show their real state. No PE parsing, no reclassification, no
scan; the status line reads `N games (cached)`. A missing, unreadable,
corrupt, stale-schema, or empty cache falls through to `Scan`. The cache is
rewritten after every scan, `AddDirectory`/`RemoveDirectory`, and op settle
(status change), serialized so concurrent writers cannot interleave.
Explicit rescans stay user-initiated (GUI Scan button, TUI `R`).

Between scans, selection is the freshness point: `Session.Select` (GUI card
and list) and the TUI's detail-open both run `RefreshInstallState`, which
re-probes the row's injection dir from disk (a few stats plus one bounded
PE identity parse) and rewrites the row — and the cache — when it drifted.
A hook installed, deleted, or renamed away by hand (the manager's
`.disabled` or any backup-style suffix like `.1`/`.bak`) thus renders
correctly on the next click without a rescan. Manifest semantics mirror the
scan: committed rows keep their manifest status (only the on-disk toggle
drifts), external rows follow the disk exactly (a hand-deleted hook clears
the install), and never-installed rows gain `external` when a branded hook
appears. Interrupted and rolled-back rows are skipped — partial files must
not flip them; the repair surface owns them.

## External install detection (v0.6)

The enrich phase derives one more status. A game with no store manifest may
still carry an OptiScaler dropped in by hand; `app.ScanAllLibraries` probes
such unmanaged rows with `pever.DetectOptiScaler(injectionDir)`. The probe
stats the injection-name candidates (dxgi.dll, OptiScaler.dll, winmm.dll,
dbghelp.dll, version.dll, wininet.dll, winhttp.dll, d3d12.dll) and accepts a
candidate only when its PE StringFileInfo (ProductName, CompanyName, or
OriginalFilename) contains "optiscaler" — identity by version info, not
filename, so renamed shims count and DXVK's dxgi.dll does not. Reads are
bounded (size cap + LimitReader, same hardening as the rest of pever), the
probe runs inside the scan goroutine, and manifests stay authoritative:
managed games are never probed. A match yields the derived status
`domain.StatusExternal` with a version from the manifest.json →
OptiScaler.log → PE FileVersion chain; component versions parse for
external rows too — those DLLs are the game's (the OptiScaler bundle
ships none of them).

The status model is 4 persisted + 1 derived: `in_progress`, `committed`,
`failed`, `rolled_back` are written to store manifests; `external` exists
only at scan time and in the `games.json` cache (warm-cache reconcile keeps
external rows; manifests override only where they exist). Session semantics:
QuickInstall on an external row is an adopt — the installer backs the
external files up SHA-verified, so uninstall/rollback restores them
byte-identically and the post-uninstall re-detect (`pever.DetectOptiScaler`
on the row's injection dir) surfaces the row as external again. Uninstall of
a never-managed external row is refused up front with a clean toast (the
`app.ErrNotManaged` sentinel never leaks raw). `GameRow.CanOpenINI()`
(committed or external) gates Open INI in both frontends.

## Hook disable/enable toggle (v0.13)

`Session.ToggleDisabled(gameDir)` parks the install's injection hook by
rename: `dxgi.dll` ↔ `dxgi.dll.disabled`. A parked hook is not loaded by
the game, so OptiScaler is off while the install stays on disk and keeps
its status. The rename is atomic and runs synchronously (no op slot, no
manifest write); the surface is a GUI button in the detail panel (the card
renders the "disabled" pill, not a button) plus the TUI `d` key, both
captioned by `GameRow.DisableToggleLabel()`.

`pever.hookCandidates` is the 8-name candidate set (dxgi, winmm, version,
dbghelp, d3d12, wininet, winhttp, OptiScaler.asi) and every probe
rescans the whole set per call, so a hook the user renamed between known
names is still found under its current name. Rename safety is asymmetric
by design: the DISABLE direction is identity-gated (`pever.ActiveHook`
parses the candidate PE for an OptiScaler marker, so a lookalike such as
DXVK's dxgi.dll is refused with a toast), while the ENABLE direction is
presence-based on established installs — a known hook name carrying the
`.disabled` suffix or any hand backup-style suffix (`.1`, `.bak`, ...)
is parked OptiScaler and is restored to its real hook name from
whichever parked file is found, the manager's own suffix winning when
several exist. The gate applies to parked hooks only at first-time
detection of UNMANAGED directories (`pever.DisabledHookVerified`), where
a DXVK `dxgi.dll.disabled` must not create an external install.

`GameRow.Disabled` renders as a "disabled" pill (GUI card badge row and
detail panel; TUI status line). The flag lives on disk, not in the
manifest, so the warm-boot reconcile and the selection-time re-probe
(Startup flow, above) both re-read it.

## NVIDIA DLSS runtime update (v0.14)

The DLSS version pill doubles as a control wherever a row reports a DLSS
component — the versioned `DLSS <version>` pill or the bare `DLSS` pill a
version-unreadable DLL degrades to (the bare label matches the card's
tech badge, so a DLL whose version resource does not parse stays
updatable instead of silently vanishing from the detail view; an
unreadable applied version simply counts as older). Pressing the version
area updates the game's NVIDIA runtime from the official NVIDIA/DLSS
repository, and the shared dropdown arrow beside it opens the restore
menu of local backups — the same popup, keyboard model, and dismissal as
the version picker (see "Shared dropdown menus" below). The control
renders on the card and in the detail panel; busy games fall back to the
static pill. Component versions parse for EVERY row with a resolved
injection dir — plain games (no OptiScaler install) and external rows
(hand-installed OptiScaler) alike: the NVIDIA runtime DLLs are the
game's, not the bundle's (OptiScaler ships only `optiscaler.dll` and
`fakenvapi.*`), and a suppressed pill would leave hand-installed games
un-updatable while their cards still showed the DLSS badge.
`classify.Dir` already walked those directories for the tech badges, and
only the few detected component DLLs get a bounded PE read. In the
detail panel the status and version-pill rows sit UNDER the 2:3 cover
art, as before; the cover scales with the panel width, so at narrow
windows the pill row can fall past the scroll fold — the panel viewport
scrolls on wheel input (with a scrollbar), which keeps every row
reachable without moving the pills off their familiar place. When the
startup check knows the published version and the applied one is
unreadable or older, the control's version segment grows a
` → <published>` marker.

## DLSS published-version check and cache-first updates (v0.14g)

On startup (when online lookups are enabled) the session makes ONE call —
GitHub's tags API for NVIDIA/DLSS (`/repos/NVIDIA/DLSS/tags?per_page=10`;
newest-first is not a documented GitHub contract, so the greatest VERSION
among the fetched tags wins, each carrying its commit SHA) — and stores
the published version and commit, then PRE-DOWNLOADS that latest
published set into the download cache when it is not already cached
(v0.15: the startup check fetches `latest`; zero network when the commit
sits complete in the cache) so the pill serves the latest offline-ready;
a failure stays silent (the status line just keeps its em-dashes). The TUI
detail view shows `DLSS published: <v> · cached: <v>`, and the GUI's
update marker uses the published half — or the cached half when the
online one is unknown (offline mode, failed lookup): the cache's version
then marks the pill too, so a newer cached set is never invisible.

## Startup latest pre-warm (v0.15)

At program startup (online lookups on) the session ALSO checks the
OptiScaler side: `startupPreload` (internal/ui/latest.go) resolves
`latest` to a concrete tag once, memoizes it (`LatestKnown`), and
downloads the tag's bundle into the download cache when it is not
already cached. The GUI version dropdown renders the memoized tag as a
first-class **Latest (tag)** option — it absorbs the concrete entry when
that entry IS the latest (one row, one tick) or prepends when absent —
and picking it dispatches the literal `latest`, which `SwitchVersion`
re-resolves at pick time before the chain starts (the same-version no-op
guard, the EAC consent pin, and the install leg all see the concrete
tag). The TUI (v0.16) mirrors that on the `v` version cycle with the
same composition rule: the entry semver-equal to the known latest tag is
replaced by a single `Latest (tag)` row in place (prepended when the
list carries no such entry), Enter dispatches `latest` (resolved at pick
time), and confirming an absorbed row — the current version — stays the
S13 no-op. The games-table version cell shows a short `→ Latest` while a
Latest row is staged — the full label would cut mid-name at the cell
width and read as version v0.9; the detail line keeps `Latest (tag)`.
Without a known latest (offline boot)
both frontends behave exactly as before. Both pre-warms (this one and the
DLSS one above) are async and
failure-silent: an offline boot simply leaves the menu at the concrete
cached versions and the next press resolves online as before. The gh
client is concurrency-safe (a scan and a preload can Resolve at the same
instant), and a release cache written by a LIVE fetch in this process is
provably fresh (`gh.CacheFresh`) — installs served from it skip the
stale-cache consent prompt, whose "stale" premise does not hold for data
fetched moments earlier.

On a user press the update is cache-first: `dlss.Update` takes a commit
hint — the startup check's tag commit, or the newest cached commit when
the published one is unknown — and uses it only when that commit already
sits complete in the download cache — otherwise it re-resolves `main` to
the current immutable commit as before. Before any write the press
compares the target against the installed set: byte-identical members
(the cache's own digests) or a readable applied version at or above the
target settle as a graceful no-op (`AlreadyLatestError` → an
informational "NVIDIA DLSS already at <v>" toast, no snapshot, no file
changes); an unreadable applied version is not provably current, so the
update proceeds. Install always copies FROM the cache dir, never
straight from the network. One tags call per startup, one API resolve
per uncached commit; the tag→SHA may briefly lag `main` (a tag published
before the latest commit), which only matters for a cache hit of an
already-installed set — the update marker shows nothing when applied >=
published. The TUI mirrors the check with the same one-call policy; CLI
surfaces remain deferred.

`dlss.Update` is a three-file transaction, never a per-DLL picker: it
requires the complete existing set (`nvngx_dlss.dll`, `nvngx_dlssd.dll`,
`nvngx_dlssg.dll` must all be present — the updater updates, it never
injects a component the game did not ship), fetches the three files from
`lib/Windows_x86_64/rel` at ONE immutable commit (resolved via the
GitHub API, never mutable `main` raw URLs), backs the current files up
as a snapshot, then swaps them in. Any failure or cancellation restores
the complete snapshot before returning; no partial set survives.
Downloads validate as PE images before any game-dir write — and before
the download cache records them.

Downloads are cached per commit at `<cacheDir>/dlss/<commit>/` — the
same layout as the OptiScaler bundle cache
(`<cacheDir>/optiscaler/<version>/`, fetch-once-per-version under one
cacheDir root); on top of that layout the dlss cache adds a
`manifest.json` pinning each member's SHA-256. Cached members are
re-verified against it on every update, freshly downloaded members are
PE-validated before their hash is recorded, and any missing or
mismatched member is refetched through the same routine — so a commit
that is already cached costs one tiny GitHub API call and no DLL
downloads, and a moved `main` resolves to a fresh commit dir whose
bytes are fetched (a stale cache dir is never served). Presses with a
known published commit skip even that resolve while the commit is
complete in cache (the cache-first hint above). The
cache holds only re-derivable downloads; snapshots stay under the state
root. `dlss.Restore` backs the current set up
first (so a restore is itself reversible), SHA-256 verifies every
snapshot member BEFORE the first copy, and then swaps the whole set
back.

Snapshots live at `<data-root>/dlss-backups/<sha256(installDir)[:16]>/<id>/`
with one `snapshot.json` each (created-at, per-file version + SHA-256,
source commit for update snapshots). A backup whose member digests an
existing snapshot already holds is REUSED, not duplicated — the
update/restore ping-pong cannot pile up identical ~115 MB dirs (the
current set is hashed first; a digest match returns the prior snapshot
untouched, on a miss the copy proceeds as before). They are deliberately
separate from the OptiScaler manifests: uninstalling or switching
OptiScaler never touches the game's NVIDIA runtime, and the restore menu
is the only downgrade path (no version picker, no update checks — the
action always fetches the current HEAD commit). The GUI confirm gate
reuses the session's `ConfirmDLSSRestore` kind; declining runs nothing.
The TUI mirrors the control: `u` on the games screen or detail screen
dispatches the update, and `p` on the detail screen stages a restore pick
that cycles the snapshots (enter confirm, esc cancel, same row-modal
pattern as the version cycle) before the same session confirmation gate.
The CLI exposes these ops as one-shot commands — see
[CLI surfaces (v0.16)](#cli-surfaces-v016).

Licensing: the NVIDIA/DLSS repository is distributed under NVIDIA's RTX
SDK license (not an open-source license). This manager ships no NVIDIA
bytes; it downloads them on explicit user action from NVIDIA's official
repository and stores restore copies locally. See docs/safety.md.

## CLI surfaces (v0.16)

The post-v0.1 features lived only behind the GUI/TUI; v0.16 adds five
one-shot kong commands in `cmd/` that run the SAME session core
(`newSession(d)` → `ui.NewSession` — identical wiring to the GUI/TUI
boot): `switch` (version switching incl. the literal `latest`,
re-resolved at pick time by the core's v0.15 seam; empty means the
configured default), `dlss-update`, `dlss-restore` (snapshot id
optional; default newest), `launch` (fire-and-forget request), and
`hook --enable|--disable`.

Each command boots a session (`sess.Start`), waits for the games list to
settle (`awaitRow`: a warm games cache is already there when Start
returns; a cold boot scans asynchronously and `awaitRow` waits for
`EvScanDone`/`EvScanFailed` before reporting an unknown dir), dispatches
the op, and blocks in `waitForOp` — the CLI's replacement for the
frontends' event loops, which drain `Session.Events()` (buffered, cap
64). The waiter ignores events for other game dirs, answers consent
gates inline, and returns on the op's terminal event.

Two terminal-event contracts make the one-shot wait deterministic:

- Single-leg ops (install, DLSS update/restore, launch) settle with
  exactly one `EvOpDone`/`EvOpFailed`/`EvOpCancelled`.
- The version-switch CHAIN (pre-flight → uninstall leg → install leg →
  ini write-back, plus rollback on failure) emits its sub-legs'
  `EvOpDone`/`EvOpFailed` mid-flight; its ONE terminal event is the new
  `EvOpSettled` with the outcome text ("switched to <tag>", "already at
  <tag>", "switch cancelled", "switch failed: …", "cannot resolve the
  latest OptiScaler version", "unknown game dir", "game is not
  installed; nothing to switch").
  `SwitchCmd` waits via `waitForOpKinds(…, EvOpSettled)` — sub-leg
  events are ignored — and maps the settle text to output/exit code.
  The frontends are unaffected: the GUI discards events (it re-reads
  the snapshot) and the TUI treats every event as a render poke.

Consent gates (`EvConfirm`) are answered on the terminal: `answerGate`
prints the pending message to the command's error writer and reads y/n.
A non-TTY stdin (checked with
`charmbracelet/x/term.IsTerminal`, already vendored via bubbletea)
DECLINES — the refused op never starts (the gates pause before the op
registers) and the command exits 1 with the reason. There is no `--yes`
flag anywhere: the CLI never bypasses the consent model.

`--timeout` (default 10m, single source in `opTimeout`) bounds every
wait; the `hook` command is the exception (its core rename is
synchronous — no waiter, no flag). Exit codes: 0 success, 1 runtime
failure (`cmd.ExitError`), 2 usage (kong parse — including the hook
command's missing state flag, enforced by kong's `required`+`xor` — or
a command's own deliberate `ExitError`, which `Run` passes through
unwrapped). The
`hook` toggle is the exception to the waiter: the core's rename is
atomic and synchronous, so the command compares the row's `Disabled`
before/after — already in the wanted state is a no-op report, a failed
rename exits 1.

Testability: `cmd.Deps` gained the same seams the GUI/TUI tests use —
`DLSS`, `Launcher`, and `Covers` (nil → built in `newSession`, like the
existing `GH`) and `SteamRoot` ("" → auto-detect, the GUI/TUI behavior;
session tests MUST pin it to the fixture root so a scan never touches the
real machine's libraries). The session-backed command tests also run
offline (`OnlineLookups: false` saved into the fixture settings; the
covers client points at a dead port — a cover miss is tolerated).

## Shared dropdown menus and pointer cursor (v0.14i)

Every dropdown in the GUI — the per-game OptiScaler version picker, the
toolbar sort menu, and the DLSS restore menu — renders through one
machinery (`internal/gui/dropdown.go`), extracted when the DLSS restore
menu stopped carrying its own popup copy. The trigger is the focus owner:
while its popup is open it consumes Up/Down (move the wrapping highlight),
Enter (pick the highlighted row) and Space (toggle closed); rows adopt the
highlight from the mouse only on actual mouse motion (a resting mouse
cannot fight the arrow keys); the popup renders through Popup (root scope,
so it escapes the card's Clip), anchored below the trigger and clamped to
the window; Esc closes without dispatch and is consumed before the global
Esc handler, and a click outside both trigger and popup closes without
dispatch. Each dropdown keeps its own observability seam (rendered rows
with rects and highlight state) and its own open-time highlight init
(the ticked version, the current sort mode, the newest snapshot).
Coordination: one dropdown open at a time per mechanism — the version
picker via `m.openDropdownDir`, the DLSS menu via `m.openDLSSDir` with a
per-card-instance `Use` state that clears itself when another card owns
the field.

Focus inside the grid is contextual per card: the card HOSTS the focus
ring while focus sits anywhere in its subtree — the card itself, a pill,
a button, the trigger of an open menu — so opening a menu from a pill
never blanks the parent's ring (a child must not trigger focus-ring
hiding on its parent). Hosted child controls suppress their own ring
(the version pill checks `m.cardRingOnDir`); when no card subtree holds
focus, the keyboard cursor's card wears the ring instead. Exactly one
ring is ever lit.

Hovering a click affordance — a button or a pressable pill — shows the
pointing hand, and nothing else does: shirei carries a small cursor-shape
patch (vendor patch v0.17, see docs/vendor-patches.md) whose hover-chain
rule picks the shape every frame from an explicit `PointerHand` attr (a
`TextEntry` keeps the arrow even under a `PointerHand` ancestor).
Focusability deliberately does NOT imply the hand — grid cards and other
focusable containers keep the arrow over their body — and the wayland
side applies shapes with the tracked pointer-enter serial, since button
or leave serials make the compositor silently ignore cursor updates.
The app sets `PointerHand` on the buttons (the `focusableButtonExt`/
`focusableToggle` wrappers), the pill controls (version trigger, DLSS
update/arrow segments), the sort trigger, the view switch and its
segments, sidebar items, and dropdown rows; themed inputs set
`TextEntry`. The wayland backend applies the picked shape after every
frame — before the unchanged-frame early return, so hover moves work
without a repaint — via the compositor's wp_cursor_shape protocol,
falling back to the theme's hand cursor, then the drawn arrow. X11 and
Win32 keep their static cursors.

## Game-dir classification and container scan roots (v0.7)

`discovery.ClassifyGameDir(ctx, dir)` sorts a directory into `GameDirGame`,
`GameDirContainer`, or `GameDirEmpty` using only stats and bounded directory
walks — executable candidacy, skip tokens, and the depth cap are exactly
`findMainExe`'s, so the predicate never disagrees with the scanner about
what counts as a game binary. An exe at depth ≤ 1 (in the dir or one level
down) means game; no game-bearing immediate subdirectory means empty;
exactly one gamey child reachable within two levels means an engine-style
game (Binaries/Win64); anything else is a container — a library root whose
games live one level down. `LooksLikeGameDir` is the boolean form. The
recursive scan itself skips exe-less subdirectories (T1), so containers
scanned as roots surface only real games.

The session consumes the classification in two places (T2):

- **Scan** classifies each extra root once per scan (bounded, no PE
  parsing). Roots classified container/empty are scan roots only:
  `mergeExtraDirs` appends no `ManualEntry` self-row for them, the covers
  progress total excludes them, and the in-flight merge drops stale
  self-rows left in `games.json` by pre-gating builds. Every other root
  ticks the covers progress once — row appended, deduplicated, or failed
  — so the phase always reaches its total. Roots whose classification
  fails keep the previous row-bearing behavior (conservative).
- **AddDirectory** classifies synchronously — cheap enough for an explicit
  user action — and branches: game dirs take the v0.5 async contract
  unchanged (synchronous persist + placeholder row, goroutine enrichment,
  `EvScanDone "directory added"`); containers persist as scan roots with
  no placeholder/self-row, toast "registered `<base>` as a scan folder",
  and trigger a background `Scan` (the rescan is the completion signal —
  no "directory added" text frontends could misread as a single-game
  add); empty dirs are refused with a warning toast before any settings
  mutation or op registration. A classification error falls through to the
  game flow, whose async error handling reports the problem.

## Scan phases, progress, and online lookups (v0.5)

A scan runs as a pipeline of phases — discover → enrich → covers → lookup —
and reports `State.Progress{Phase, Done, Total}` as it goes (`EvScanProgress`
events); the GUI draws a progress bar under the toolbar, the TUI a progress
line. The lookup phase is online and optional: `internal/steam` resolves a
manual game's title to a Steam appid and `internal/protondb` resolves the
appid to a compatibility tier (Steam-library rows skip the search and query
the tier directly). It runs under a per-scan budget (8 rows), TTL disk
caches, and a 429 cooldown, degrades silently when offline, and is gated by
`online_lookups` (default true) — with either client nil or the setting off,
the phase is skipped entirely.

`AddDirectory` is asynchronous by design: the session validates the path,
persists settings, and inserts a placeholder row synchronously, then a
goroutine walks, classifies, covers, and online-enriches the directory and
replaces the placeholder. A duplicate add while one is in flight is
rejected. `ClearBundleCache` likewise runs off the frame goroutine.

## Data flow

```
scan/install (CLI) ─┐
                    ├─→ discovery → classify → gh → archive → installer → store
GUI/TUI (Session) ──┘        (domain packages never import shirei)
                 ↕ covers (Steam CDN / store search)

ui.Session: commands spawn goroutines; state mutated under its mutex;
frontends drain Events() and render Snapshot(). shirei rule: all UI calls on
the frame goroutine; the GUI binding drains events each frame (non-blocking)
and re-renders.
```

## External state root

Manifests and backups live outside game directories under the platform data
dir (XDG: `$XDG_DATA_HOME/optiscaler-manager` or `~/.local/share/...`),
overridable for tests. Game directories are never used as our database.

## Why bundle-only

OptiScaler ≥ 0.9 bundles every satellite component into one archive. Treating
"OptiScaler" as one artifact removes the C# client's hardest problems
(version matrices, per-component caches, bundled-vs-separate toggles) and
collapses its six download pipelines into one.

### v0.8 game identification

`internal/gid` sits below discovery and owns offline identification:
hard-ID file readers (steam_appid.txt, goggame, .egstore, Unity
app.info) and the normalized fuzzy scorer. Layering: `ui → app →
discovery → gid → {domain, pever}`; the GOG/Epic manifest types moved
there with thin aliases left in discovery.

Row creation resolves titles through `discovery.ChainResolver`:
settings override → gid offline hits → PE → exe stem → folder, always
recording a detected Steam appid. The online half lives in the lookup
phase (`ui/identify.go`): appid rows upgrade to canonical store names
(appdetails), the rest try the fuzzy store match (storesearch, with
SearchApps → ProtonDB behind it), all under the existing budget,
pacing, cooldown, and disk caches.

TitleSource contract: it always names the rule that produced the
CURRENT displayed title — an offline scan with a detected appid keeps
`pe`/`stem`/`folder` (never `storeid` next to a tail title), and the
online upgrade flips it to `storeid`/`fuzzy`. Override rows are frozen;
store-manifest rows carry no source.
