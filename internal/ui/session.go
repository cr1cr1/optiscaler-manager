package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/launch"
	"github.com/cr1cr1/optiscaler-manager/internal/pcgw"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/pickdir"
	"github.com/cr1cr1/optiscaler-manager/internal/protondb"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/steam"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
	"github.com/cr1cr1/optiscaler-manager/internal/termopen"
)

// toastTTL is how long a toast stays visible.
const toastTTL = 8 * time.Second

// ViewMode selects the library presentation.
type ViewMode int

const (
	ViewGrid ViewMode = iota
	ViewList
)

// EventKind identifies a session notification.
type EventKind int

const (
	EvScanStarted EventKind = iota
	EvScanDone
	EvScanFailed
	EvOpStarted
	EvOpDone
	EvOpFailed
	EvOpCancelled
	EvConfirm
	EvScanProgress
)

// Scan progress phases, in pipeline order. "covers" includes the manual
// extra-dir rows merged after the discovered entries.
const (
	phaseDiscover = "discover"
	phaseEnrich   = "enrich"
	phaseCovers   = "covers"
	phaseLookup   = "lookup"
)

// progressPokeInterval is the minimum spacing between EvScanProgress pokes
// within a phase; the State.Progress snapshot is updated on every tick
// regardless.
const progressPokeInterval = 50 * time.Millisecond

// ScanProgress is one pipeline phase's completion counters.
type ScanProgress struct {
	Phase string
	Done  int
	Total int
}

// Event is a single notification on the session's event stream.
type Event struct {
	Kind    EventKind
	Text    string
	GameDir string
}

// ConfirmKind identifies what the session needs consent for.
type ConfirmKind int

const (
	ConfirmEAC ConfirmKind = iota
	ConfirmCachedRelease
)

// Confirmation is a pending consent request. Installs never proceed past
// these points until AnswerConfirm(true) — the frontend renders the prompt.
type Confirmation struct {
	Kind    ConfirmKind
	GameDir string
	Message string
}

// Toast is a transient notification.
type Toast struct {
	Text    string
	Warn    bool
	AddedAt time.Time
}

// State is the renderable snapshot of the session.
type State struct {
	Rows       []GameRow
	Query      string
	Mode       ViewMode
	Sort       SortMode
	Selected   string
	Busy       string // op description, "" when idle
	StatusLine string
	Confirm    *Confirmation
	Toasts     []Toast
	Progress   *ScanProgress // scan pipeline counters, nil when no scan runs
}

// Deps wires the session to the lower layers.
type Deps struct {
	Store        *store.Store
	GH           *gh.Client
	Covers       *covers.Covers
	CacheDir     string
	SteamRoot    string
	Settings     settings.Settings
	SettingsRoot string
	Launcher     *launch.Launcher // nil selects the platform detached-spawn default

	// Steam and ProtonDB feed the online lookup phase of Scan; either nil
	// skips enrichment entirely.
	Steam    *steam.Client
	ProtonDB *protondb.Client

	// PCGW is the secondary canonical-title source (PCGamingWiki), used
	// when Steam's storesearch finds nothing; nil disables the fallback.
	PCGW *pcgw.Client

	// GOOS selects the target platform behavior (empty = runtime.GOOS);
	// ProtonDB enrichment and cached proton tiers are linux-only.
	GOOS string
}

// Session is the frontend-agnostic interactive core.
type Session struct {
	deps   Deps
	events chan Event

	mu        sync.Mutex
	st        State
	now       func() time.Time
	opCancels map[string]context.CancelFunc // in-flight op per game dir
	cacheMu   sync.Mutex                    // serializes games-cache writes

	progressMu sync.Mutex // serializes progress poke throttling
	lastPoke   time.Time

	scanMu      sync.Mutex // guards scanning/scanPending
	scanning    bool       // a scan goroutine is running
	scanPending bool       // a Scan landed mid-scan; the running scan re-runs once

	openExternal func(path string) error
	pickDir      func(ctx context.Context) (string, error)
	removeAll    func(path string) error

	// resolveVersion is the test seam for resolving the configured default
	// version to a concrete tag (fresh = live fetch, not cache); nil picks
	// ghResolveVersion. Set it before the first Scan, like openExternal.
	resolveVersion func(ctx context.Context, requested string) (version string, fresh bool, err error)

	// upgradeGapHook is a test seam invoked synchronously between the
	// uninstall and install legs of a committed-row upgrade (see
	// doUpgrade); nil in production. It lets a test occupy the game's op
	// slot in the finishOp→registerOp gap so the install leg's errOpBusy
	// path is exercised deterministically.
	upgradeGapHook func(gameDir string)

	// resolvedDefault* memoize the default-version resolution (see
	// upgrade.go): one Resolve per distinct configured value, never per
	// row or per frame.
	resolvedDefaultKey     string
	resolvedDefaultVersion string
	resolvedDefaultFresh   bool
	resolvedDefaultAt      time.Time
}

// NewSession starts a session. The library is empty until Scan is called.
// The settings root is created up front so later background writers never
// need to recreate directories.
func NewSession(deps Deps) *Session {
	if deps.Settings.DefaultVersion == "" {
		deps.Settings.DefaultVersion = "latest"
	}
	if deps.Settings.LaunchTemplate == "" {
		deps.Settings.LaunchTemplate = settings.DefaultLaunchTemplate
	}
	if deps.Launcher == nil {
		deps.Launcher = launch.New(nil, "", nil)
	}
	if deps.GOOS == "" {
		deps.GOOS = runtime.GOOS
	}
	if deps.SettingsRoot != "" {
		if err := os.MkdirAll(deps.SettingsRoot, 0o755); err != nil {
			log.Warn().Err(err).Str("root", deps.SettingsRoot).Msg("settings root not creatable")
		}
	}
	return &Session{
		deps:         deps,
		events:       make(chan Event, 64),
		st:           State{Mode: ViewGrid, StatusLine: "Ready"},
		now:          time.Now,
		opCancels:    map[string]context.CancelFunc{},
		openExternal: openExternal,
		pickDir:      pickdir.Pick,
		removeAll:    os.RemoveAll,
	}
}

// openExternal opens path with the platform's default handler. On Linux
// that is a terminal editor ($EDITOR → micro → nano → vi) running inside a
// terminal emulator, spawned detached; darwin and windows keep the OS
// file handler.
func openExternal(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	default: // linux and the rest
		return termopen.New("", nil, nil, nil).Open(path)
	}
}

// Settings returns a snapshot of the current user settings; the ExtraDirs
// slice is deep-copied so callers may iterate it while mutators run.
func (s *Session) Settings() settings.Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.deps.Settings
	out.ExtraDirs = append([]string(nil), out.ExtraDirs...)
	return out
}

// SetDefaultVersion changes the release tag installs resolve to (persisted).
func (s *Session) SetDefaultVersion(v string) {
	if v == "" {
		v = "latest"
	}
	s.mu.Lock()
	s.deps.Settings.DefaultVersion = v
	snap := s.deps.Settings
	s.mu.Unlock()
	if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
		s.toast("settings not saved: "+err.Error(), true)
		return
	}
	s.toast("default version: "+v, false)
}

// SetOnlineLookups toggles ProtonDB/Steam game-info enrichment during
// scans (persisted); frontends render it as the online-lookups switch.
func (s *Session) SetOnlineLookups(v bool) {
	s.mu.Lock()
	s.deps.Settings.OnlineLookups = v
	snap := s.deps.Settings
	s.mu.Unlock()
	if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
		s.toast("settings not saved: "+err.Error(), true)
		return
	}
	if v {
		s.toast("online lookups: on", false)
	} else {
		s.toast("online lookups: off", false)
	}
}

// SetLaunchTemplate changes the command template manual games launch with
// (persisted); an empty value resets to the plain `"{exe}" {args}` default.
func (s *Session) SetLaunchTemplate(tmpl string) {
	if tmpl == "" {
		tmpl = settings.DefaultLaunchTemplate
	}
	s.mu.Lock()
	s.deps.Settings.LaunchTemplate = tmpl
	snap := s.deps.Settings
	s.mu.Unlock()
	if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
		s.toast("settings not saved: "+err.Error(), true)
		return
	}
	s.toast("launch template: "+tmpl, false)
}

// ClearBundleCache deletes all cached OptiScaler bundles. The deletion runs
// in the background (large caches can take a while); a toast reports the
// outcome.
func (s *Session) ClearBundleCache() {
	dir := filepath.Join(s.deps.CacheDir, "optiscaler")
	go func() {
		if err := s.removeAll(dir); err != nil {
			s.toast("clear cache: "+err.Error(), true)
			return
		}
		s.toast("OptiScaler cache cleared", false)
	}()
}

// Events returns the notification stream. Frontends drain it (GUI: each
// frame, non-blocking; TUI: blocking) and re-render from Snapshot.
func (s *Session) Events() <-chan Event {
	return s.events
}

// Snapshot returns the current renderable state, pruning expired toasts.
func (s *Session) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := s.now().Add(-toastTTL)
	kept := s.st.Toasts[:0]
	for _, t := range s.st.Toasts {
		if t.AddedAt.After(cutoff) {
			kept = append(kept, t)
		}
	}
	s.st.Toasts = kept
	out := s.st
	out.Rows = append([]GameRow(nil), s.st.Rows...)
	out.Toasts = append([]Toast(nil), s.st.Toasts...)
	return out
}

// VisibleRows returns the rows after the current query filter and sort.
func (s *Session) VisibleRows() []GameRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := filterRows(append([]GameRow(nil), s.st.Rows...), s.st.Query)
	if s.st.Sort == SortName {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Title < rows[j].Title })
	}
	return rows
}

// SetQuery updates the search filter (cheap, synchronous).
func (s *Session) SetQuery(q string) {
	s.mu.Lock()
	s.st.Query = q
	s.mu.Unlock()
}

// SetSort selects the row ordering VisibleRows applies; out-of-range modes
// reset to SortDefault. Cheap and synchronous like SetQuery.
func (s *Session) SetSort(mode SortMode) {
	if mode != SortName {
		mode = SortDefault
	}
	s.mu.Lock()
	s.st.Sort = mode
	s.mu.Unlock()
}

// ToggleView flips between grid and list presentation.
func (s *Session) ToggleView() {
	s.mu.Lock()
	if s.st.Mode == ViewGrid {
		s.st.Mode = ViewList
	} else {
		s.st.Mode = ViewGrid
	}
	s.mu.Unlock()
}

// Select marks a game as the dashboard target ("" closes it).
func (s *Session) Select(dir string) {
	s.mu.Lock()
	s.st.Selected = dir
	s.mu.Unlock()
}

// Start boots the library: a warm games cache hydrates the rows
// synchronously (status reconciled from store manifests — no PE parsing, no
// reclassification) and no scan runs; a missing or unusable cache falls
// through to Scan. Safe to call once at frontend boot.
func (s *Session) Start(ctx context.Context) {
	rows := loadGamesCache(s.deps.SettingsRoot, s.deps.GOOS)
	if len(rows) == 0 {
		s.Scan(ctx)
		return
	}
	s.reconcileStatuses(rows)
	sortRows(rows)
	s.mu.Lock()
	s.st.Rows = rows
	s.st.StatusLine = fmt.Sprintf("%d games (cached)", len(rows))
	s.st.Busy = ""
	s.mu.Unlock()
}

// reconcileStatuses overrides cached row status from store manifests keyed
// by canonical install dir (and game root), so installs that settled while
// the manager was not running show their real state.
func (s *Session) reconcileStatuses(rows []GameRow) {
	if s.deps.Store == nil {
		return
	}
	manifests, err := s.deps.Store.List()
	if err != nil {
		log.Warn().Err(err).Msg("games cache: status reconcile skipped")
		return
	}
	byDir := map[string]domain.Status{}
	byRoot := map[string]domain.Status{}
	for _, m := range manifests {
		byDir[m.InstallDir] = m.Status
		byRoot[m.GameRoot] = m.Status
	}
	for i := range rows {
		st, ok := byDir[rows[i].InjectionDir]
		if !ok {
			st, ok = byRoot[rows[i].InstallDir]
		}
		if ok {
			rows[i].Status = st
			rows[i].Actionable = actionableStatus(st)
		}
	}
}

// Scan refreshes the library asynchronously. Scans are serialized: a Scan
// landing while one is in flight sets a pending bit instead of spawning a
// second goroutine, and the running scan re-runs once when it settles
// (success or failure). A container added mid-scan is therefore surfaced by
// a scan whose settings snapshot already includes it — an earlier scan
// settling last can no longer wipe freshly surfaced rows — and concurrent
// scans never thrash the busy/progress state. Only scans that actually run
// emit EvScanStarted/EvScanDone; a coalesced call emits nothing.
func (s *Session) Scan(ctx context.Context) {
	s.scanMu.Lock()
	if s.scanning {
		s.scanPending = true
		s.scanMu.Unlock()
		return
	}
	s.scanning = true
	s.scanMu.Unlock()
	go func() {
		for {
			s.runScan(ctx)
			s.scanMu.Lock()
			if !s.scanPending {
				s.scanning = false
				s.scanMu.Unlock()
				return
			}
			s.scanPending = false
			s.scanMu.Unlock()
			// The pending re-run's trigger may outlive the caller's ctx.
			ctx = context.Background()
		}
	}()
}

// scanIdle reports whether no scan is running and none is pending.
func (s *Session) scanIdle() bool {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	return !s.scanning && !s.scanPending
}

// runScan performs one scan pass, emitting EvScanStarted up front and
// EvScanDone/EvScanFailed on settle.
func (s *Session) runScan(ctx context.Context) {
	s.emit(Event{Kind: EvScanStarted})
	s.setBusy("Scanning…")
	s.resetProgress()
	snap := s.Settings()
	// Resolve the default OptiScaler version once per scan (never per row
	// or frame); toRow compares every installed row against the memo.
	// Gated on online lookups like the rest of the scan's network work.
	if snap.OnlineLookups {
		s.refreshResolvedDefault(ctx, snap.DefaultVersion)
	}
	resolver := discovery.ChainResolver(func(dir string) string {
		return snap.TitleOverrides[canonicalDir(dir)]
	})
	entries, err := app.ScanAllLibraries(ctx, s.deps.Store, app.ScanAllOptions{
		SteamRoot: s.deps.SteamRoot,
		ExtraDirs: snap.ExtraDirs,
		Progress: func(phase string, done, total int) {
			if ctx.Err() != nil {
				return
			}
			s.scanProgress(phase, done, total)
		},
		Resolver: resolver,
	})
	if err != nil {
		if errors.Is(err, app.ErrNoGames) {
			entries = nil // empty first-run library: settle at 0 games
		} else {
			s.clearProgress()
			s.setBusy("")
			s.setStatus("Scan failed: " + err.Error())
			log.Warn().Err(err).Msg("scan failed")
			s.emit(Event{Kind: EvScanFailed, Text: err.Error()})
			return
		}
	}
	// Classify each extra root once (stats and bounded walks only, no PE
	// parsing): container/empty roots are scan roots whose games already
	// surfaced via the recursive scan — they get no self-row from
	// mergeExtraDirs, no cover tick, and stale self-rows are not
	// resurrected by the in-flight keep below. Roots that fail
	// classification keep the previous row-bearing behavior.
	scanOnlyRoots := map[string]bool{}
	for _, d := range snap.ExtraDirs {
		kind, err := discovery.ClassifyGameDir(ctx, d)
		if err == nil && kind != discovery.GameDirGame {
			scanOnlyRoots[d] = true
		}
	}
	coversTotal := len(entries) + len(snap.ExtraDirs) - len(scanOnlyRoots)
	coversDone := 0
	coversTick := func() {
		coversDone++
		s.scanProgress(phaseCovers, coversDone, coversTotal)
	}
	rows := make([]GameRow, 0, len(entries))
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			s.clearProgress()
			s.setBusy("")
			s.setStatus("Scan failed: " + err.Error())
			log.Warn().Err(err).Msg("scan cancelled")
			s.emit(Event{Kind: EvScanFailed, Text: err.Error()})
			return
		}
		rows = append(rows, s.toRow(ctx, e))
		coversTick()
	}
	rows = s.mergeExtraDirs(ctx, rows, snap.ExtraDirs, scanOnlyRoots, coversTick, resolver)
	// Online lookup phase: enrich the local rows before they are
	// committed to state, so the final persistCache lands the enriched
	// fields in one write.
	s.enrichOnline(ctx, rows, snap)
	s.mu.Lock()
	// Directories added while this scan was in flight are not in the
	// snapshot's ExtraDirs; keep their rows rather than wiping them.
	// Rows for roots the snapshot classified as container/empty are
	// stale self-rows from before the gate existed — drop them.
	fresh := map[string]bool{}
	for _, r := range rows {
		fresh[r.InstallDir] = true
	}
	for _, r := range s.st.Rows {
		if fresh[r.InstallDir] {
			continue
		}
		for _, d := range s.deps.Settings.ExtraDirs {
			if r.InstallDir == d && !scanOnlyRoots[d] {
				rows = append(rows, r)
				break
			}
		}
	}
	sortRows(rows)
	disambiguateTitles(rows)
	s.refreshCovers(ctx, rows)
	s.st.Rows = rows
	s.st.StatusLine = fmt.Sprintf("%d games", len(rows))
	s.st.Busy = ""
	s.st.Progress = nil
	s.mu.Unlock()
	s.persistCache()
	s.emit(Event{Kind: EvScanDone, Text: fmt.Sprintf("%d games", len(rows))})
}

// scanProgress records one pipeline tick. State.Progress is authoritative
// and updated on every tick; an EvScanProgress poke is emitted at most
// every progressPokeInterval or on a phase change.
func (s *Session) scanProgress(phase string, done, total int) {
	s.mu.Lock()
	changed := s.st.Progress == nil || s.st.Progress.Phase != phase
	s.st.Progress = &ScanProgress{Phase: phase, Done: done, Total: total}
	s.mu.Unlock()
	s.progressMu.Lock()
	due := changed || time.Since(s.lastPoke) >= progressPokeInterval
	if due {
		s.lastPoke = time.Now()
	}
	s.progressMu.Unlock()
	if due {
		s.emit(Event{Kind: EvScanProgress})
	}
}

// resetProgress arms the poke throttle for a new scan.
func (s *Session) resetProgress() {
	s.progressMu.Lock()
	s.lastPoke = time.Time{}
	s.progressMu.Unlock()
	s.mu.Lock()
	s.st.Progress = nil
	s.mu.Unlock()
}

// clearProgress drops the progress snapshot; callers invoke it before the
// terminal scan event so frontends observe nil on completion.
func (s *Session) clearProgress() {
	s.mu.Lock()
	s.st.Progress = nil
	s.mu.Unlock()
}

// QuickInstall installs when not installed, uninstalls when installed —
// the one-click toggle from the reference client, with our defaults. An
// upgrade-eligible row is UPGRADED instead: the plain toggle would
// uninstall a committed row and leave the game with no OptiScaler at all.
func (s *Session) QuickInstall(gameDir string) {
	row := s.findRow(gameDir)
	if row != nil && row.UpgradeAvailable {
		go s.doUpgrade(gameDir)
		return
	}
	if row != nil && row.Status == domain.StatusCommitted {
		go s.doUninstall(gameDir)
		return
	}
	go s.doInstall(gameDir, false, false)
}

// Install starts an install with an explicit EAC override decision.
func (s *Session) Install(gameDir string) {
	go s.doInstall(gameDir, false, false)
}

// Uninstall starts an uninstall.
func (s *Session) Uninstall(gameDir string) {
	go s.doUninstall(gameDir)
}

// Rollback starts a rollback of an interrupted/failed install.
func (s *Session) Rollback(gameDir string) {
	go s.runRollback(gameDir)
}

// runRollback is Rollback's body, callable synchronously by chained ops
// (the upgrade chain runs it as cleanup after a failed install leg).
func (s *Session) runRollback(gameDir string) {
	pre := preOpStatus(s.findRow(gameDir))
	ctx, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return
	}
	s.opStarted("Rolling back…")
	dir, err := app.Rollback(ctx, s.deps.Store, gameDir)
	s.finishOp(gameDir)
	if errors.Is(err, context.Canceled) {
		s.opCancelled(gameDir, pre)
		return
	}
	if errors.Is(err, app.ErrNotManaged) {
		s.opRefused(errNotManagedToast, gameDir)
		return
	}
	if err != nil {
		s.opFailed(err)
		return
	}
	// Rollback restores whatever the adopt-time backup held — possibly a
	// pre-existing external install. Re-probe like doUninstall: external
	// when a branded injection DLL is back, rolled_back otherwise.
	status := domain.StatusRolledBack
	if row := s.findRow(gameDir); row != nil && redetectExternal(row.InjectionDir) == domain.StatusExternal {
		status = domain.StatusExternal
		// The rollback's idempotent job is done: drop the rolled_back
		// manifest so the next scan's enrich probe and the warm-cache
		// reconcile converge on external (exactly like the uninstall
		// path, which deletes its manifest). A later manual Rollback
		// re-run refuses cleanly via ErrNotManaged.
		id, _, err := app.ManifestIDFor(gameDir)
		if err != nil {
			log.Warn().Err(err).Str("dir", gameDir).Msg("rollback: resolve manifest id")
		} else if err := s.deps.Store.Delete(id); err != nil {
			log.Warn().Err(err).Str("id", id).Msg("rollback: drop rolled_back manifest")
		}
	}
	s.setRowStatus(gameDir, status)
	s.opDone("Rolled back "+dir, gameDir)
}

// AddDirectory registers a user-picked directory and persists it in
// settings so later scans keep it. The directory is classified up front
// (stats and bounded walks only, no PE parsing — cheap enough for an
// explicit user action) and the kind decides the flow:
//
//   - game: the call never blocks on enrichment — validation, settings
//     persistence, and a placeholder row are synchronous (so a Scan started
//     right after sees the directory), while the walk/classify/cover
//     enrichment runs in a goroutine that replaces the placeholder and
//     settles with the usual EvScanDone "directory added" event;
//   - container: registered as a scan root (settings persisted
//     synchronously) with no placeholder or self-row, a "scan folder"
//     toast, and a background rescan that surfaces its games as rows;
//   - empty: refused with a warning toast; settings stay untouched.
//
// A classification failure falls through to the game flow, whose async
// error handling reports the problem. A duplicate Add of the same
// canonical dir while one is in flight is rejected with a toast and no
// event.
func (s *Session) AddDirectory(dir string) {
	root, err := canonicalDirChecked(dir)
	if err != nil {
		s.toast("add directory: "+err.Error(), true)
		return
	}
	kind, kerr := discovery.ClassifyGameDir(context.Background(), root)
	if kerr == nil {
		switch kind {
		case discovery.GameDirEmpty:
			s.toast("no games found under "+filepath.Base(root), true)
			return
		case discovery.GameDirContainer:
			s.addScanRoot(root)
			return
		}
	}
	ctx, ok := s.registerOp(root)
	if !ok {
		s.toast("add already in progress", true)
		return
	}
	s.mu.Lock()
	exists := false
	for _, r := range s.st.Rows {
		if r.InstallDir == root {
			exists = true
			break
		}
	}
	if exists {
		s.mu.Unlock()
		s.finishOp(root)
		s.toast(filepath.Base(root)+" already in library", false)
		return
	}
	present := false
	for _, d := range s.deps.Settings.ExtraDirs {
		if d == root {
			present = true
			break
		}
	}
	if !present {
		s.deps.Settings.ExtraDirs = append(s.deps.Settings.ExtraDirs, root)
	}
	snap := s.deps.Settings
	base := filepath.Base(root)
	s.st.Rows = append(s.st.Rows, GameRow{
		Title:      base,
		AppID:      "custom_" + base,
		InstallDir: root,
		Platform:   domain.StoreManual.String(),
		Store:      domain.StoreManual,
	})
	sortRows(s.st.Rows)
	s.mu.Unlock()
	if !present {
		if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
			s.toast("settings not saved: "+err.Error(), true)
		}
	}
	go func() {
		snap := s.Settings()
		resolver := discovery.ChainResolver(func(d string) string {
			return snap.TitleOverrides[canonicalDir(d)]
		})
		entry, err := app.ManualEntryWithResolver(dir, s.deps.Store, resolver)
		if err != nil {
			s.finishOp(root)
			s.removeRow(root)
			s.toast("add directory: "+err.Error(), true)
			return
		}
		row := s.toRow(ctx, entry)
		if ctx.Err() != nil {
			s.finishOp(root)
			return // cancelled mid-add: the placeholder row stays for the next scan
		}
		s.mu.Lock()
		// A RemoveDirectory that landed while this add was enriching has
		// already dropped the placeholder row and the ExtraDirs entry;
		// re-appending now would resurrect a zombie row that survives until
		// the next scan. Skip the append, the cache write, and the event.
		registered := false
		for _, d := range s.deps.Settings.ExtraDirs {
			if d == root {
				registered = true
				break
			}
		}
		if !registered {
			s.mu.Unlock()
			s.finishOp(root)
			log.Debug().Str("dir", root).Msg("add settled after directory removal; row not appended")
			return
		}
		replaced := false
		for i := range s.st.Rows {
			if s.st.Rows[i].InstallDir == entry.Game.InstallDir {
				s.st.Rows[i] = row
				replaced = true
				break
			}
		}
		if !replaced {
			s.st.Rows = append(s.st.Rows, row)
		}
		sortRows(s.st.Rows)
		s.mu.Unlock()
		s.finishOp(root)
		s.persistCache()
		s.toast("added "+entry.Game.Name, false)
		s.emit(Event{Kind: EvScanDone, Text: "directory added"})
	}()
}

// addScanRoot registers a container directory as a recursive scan root:
// persisted in settings like a game add, but with no placeholder or
// self-row — the root's games surface as children of the background rescan
// it triggers (the rescan is the "directory added" equivalent: no
// EvScanDone text frontends could misread as a single-game add).
func (s *Session) addScanRoot(root string) {
	s.mu.Lock()
	present := false
	for _, d := range s.deps.Settings.ExtraDirs {
		if d == root {
			present = true
			break
		}
	}
	if !present {
		s.deps.Settings.ExtraDirs = append(s.deps.Settings.ExtraDirs, root)
	}
	snap := s.deps.Settings
	s.mu.Unlock()
	if !present {
		if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
			s.toast("settings not saved: "+err.Error(), true)
		}
	}
	s.toast("registered "+filepath.Base(root)+" as a scan folder", false)
	s.Scan(context.Background())
}

// removeRow drops the row for dir.
func (s *Session) removeRow(dir string) {
	s.mu.Lock()
	kept := make([]GameRow, 0, len(s.st.Rows))
	for _, r := range s.st.Rows {
		if r.InstallDir != dir {
			kept = append(kept, r)
		}
	}
	s.st.Rows = kept
	s.mu.Unlock()
}

// canonicalDirChecked canonicalizes p like canonicalDir and verifies it is
// an existing directory, mirroring the validation app.ManualEntry applies.
func canonicalDirChecked(p string) (string, error) {
	root := canonicalDir(p)
	st, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", root, err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", root)
	}
	return root, nil
}

// RemoveDirectory unregisters a manually added directory: its row and any
// nested games scanned under it are dropped, settings persist without it,
// and the games cache is rewritten. Directories not in ExtraDirs are a
// silent no-op (no write, no event).
func (s *Session) RemoveDirectory(dir string) {
	root := canonicalDir(dir)
	s.mu.Lock()
	present := false
	for _, d := range s.deps.Settings.ExtraDirs {
		if d == root {
			present = true
			break
		}
	}
	s.mu.Unlock()
	if !present {
		return
	}
	// Abort any in-flight AddDirectory for this root before dropping the
	// row: its goroutine checks ctx.Err() before touching rows, so a cancel
	// here stops it from resurrecting the row after RemoveDirectory returns.
	s.CancelOp(root)
	s.mu.Lock()
	present = false
	kept := make([]string, 0, len(s.deps.Settings.ExtraDirs))
	for _, d := range s.deps.Settings.ExtraDirs {
		if d == root {
			present = true
			continue
		}
		kept = append(kept, d)
	}
	if !present {
		s.mu.Unlock()
		return // raced with a concurrent removal of the same root
	}
	s.deps.Settings.ExtraDirs = kept
	prefix := root + string(os.PathSeparator)
	rows := s.st.Rows[:0]
	for _, r := range s.st.Rows {
		if r.InstallDir == root || strings.HasPrefix(r.InstallDir, prefix) {
			continue
		}
		rows = append(rows, r)
	}
	s.st.Rows = rows
	snap := s.deps.Settings
	s.mu.Unlock()
	if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
		s.toast("settings not saved: "+err.Error(), true)
		return
	}
	s.persistCache()
	s.toast("removed "+filepath.Base(root), false)
	s.emit(Event{Kind: EvScanDone, Text: "directory removed"})
}

// PickAndAddDirectory opens the OS directory picker and adds the choice.
func (s *Session) PickAndAddDirectory(ctx context.Context) {
	go func() {
		dir, err := s.pickDir(ctx)
		if err != nil {
			s.toast(err.Error(), true)
			return
		}
		if dir == "" {
			return // cancelled
		}
		s.AddDirectory(dir)
	}()
}

// mergeExtraDirs appends rows for game directories the user added manually
// and the scan did not surface. Roots in scanOnly (classified container or
// empty by the caller) are scan roots, not games: they get no self-row.
// extraDirs must be a locked settings snapshot taken by the caller. tick,
// when non-nil, runs once per non-scanOnly root (row appended, deduplicated,
// or failed — the caller's total counts all of them).
func (s *Session) mergeExtraDirs(ctx context.Context, rows []GameRow, extraDirs []string, scanOnly map[string]bool, tick func(), res discovery.TitleResolver) []GameRow {
	for _, d := range extraDirs {
		if scanOnly[d] {
			continue
		}
		entry, err := app.ManualEntryWithResolver(d, s.deps.Store, res)
		if err != nil {
			log.Warn().Err(err).Str("dir", d).Msg("extra dir unavailable")
		} else {
			dup := false
			for _, r := range rows {
				if r.InstallDir == entry.Game.InstallDir {
					dup = true
					break
				}
			}
			if !dup {
				rows = append(rows, s.toRow(ctx, entry))
			}
		}
		if tick != nil {
			tick()
		}
	}
	return rows
}

// AnswerConfirm resolves a pending confirmation. Accepted confirmations
// resume the operation with the consented override.
func (s *Session) AnswerConfirm(accept bool) {
	s.mu.Lock()
	c := s.st.Confirm
	s.st.Confirm = nil
	s.mu.Unlock()
	if c == nil || !accept {
		return
	}
	switch c.Kind {
	case ConfirmEAC:
		go s.doInstall(c.GameDir, true, false)
	case ConfirmCachedRelease:
		go s.doInstall(c.GameDir, false, true)
	}
}

// INIPath returns the game's OptiScaler.ini path, or "" when the game has
// no OptiScaler install (managed or external). Pure resolver with no side
// effects — the GUI's external opener and the TUI's in-process editor both
// build on it.
func (s *Session) INIPath(gameDir string) string {
	row := s.findRow(gameDir)
	if row == nil || row.InjectionDir == "" {
		return ""
	}
	return filepath.Join(row.InjectionDir, "OptiScaler.ini")
}

// OpenINI opens the game's OptiScaler.ini in the system editor (GUI path:
// a terminal editor inside a terminal emulator on Linux via termopen).
func (s *Session) OpenINI(gameDir string) {
	path := s.INIPath(gameDir)
	if path == "" {
		s.toast("no OptiScaler.ini (not installed?)", true)
		return
	}
	if err := s.openExternal(path); err != nil {
		s.toast("cannot open editor: "+err.Error(), true)
	}
}

// Toast surfaces a short message in the active frontend; the TUI uses it
// to report outcomes of work it drives itself (the in-process editor).
func (s *Session) Toast(text string, warn bool) { s.toast(text, warn) }

// Launch requests a fire-and-forget game launch: it never blocks, never
// waits on the child, and a successful request proves nothing about the
// game actually running — hence "Launch requested", never "launched".
func (s *Session) Launch(gameDir string) {
	go s.doLaunch(gameDir)
}

func (s *Session) doLaunch(gameDir string) {
	row := s.findRow(gameDir)
	if row == nil {
		s.launchFailed(fmt.Errorf("unknown game %s", gameDir), gameDir)
		return
	}
	target, err := s.launchTarget(row)
	if err != nil {
		s.launchFailed(err, gameDir)
		return
	}
	if err := s.deps.Launcher.Launch(context.Background(), target); err != nil {
		s.launchFailed(err, gameDir)
		return
	}
	what := "Launch requested: " + row.Title
	s.setStatus(what)
	s.toast(what, false)
	s.emit(Event{Kind: EvOpDone, Text: what, GameDir: gameDir})
}

func (s *Session) launchFailed(err error, gameDir string) {
	s.setStatus("Launch failed: " + err.Error())
	s.toast("Launch failed: "+err.Error(), true)
	s.emit(Event{Kind: EvOpFailed, Text: err.Error(), GameDir: gameDir})
}

// launchTarget maps a row to its launch target; manual games launch through
// the user's template, and a blank ExePath on an exe-launched store falls
// back to the discovery exe picking before giving up.
func (s *Session) launchTarget(row *GameRow) (launch.Target, error) {
	t := launch.Target{
		Store:   launchStore(row.Store),
		Name:    row.Title,
		AppID:   row.AppID,
		AppName: row.AppName,
		ExePath: row.ExePath,
		Dir:     row.InstallDir,
	}
	if t.Store == launch.StoreManual {
		t.Template = s.Settings().LaunchTemplate
	}
	if t.ExePath == "" && (t.Store == launch.StoreManual || t.Store == launch.StoreGOG) {
		exe, err := resolveGameExe(row.InstallDir)
		if err != nil {
			return t, err
		}
		t.ExePath = exe
	}
	return t, nil
}

func launchStore(s domain.Store) launch.Store {
	switch s {
	case domain.StoreEpic:
		return launch.StoreEpic
	case domain.StoreGOG:
		return launch.StoreGOG
	case domain.StoreManual:
		return launch.StoreManual
	case domain.StoreSteam:
		return launch.StoreSteam
	default:
		return launch.StoreUnknown
	}
}

// resolveGameExe reuses the recursive scanner's exe picking for one game
// directory whose ExePath discovery left blank. When the parent scan
// yields nothing (e.g. the parent dir is engine-named and refused as a
// scan root), the game's own directory is searched directly.
func resolveGameExe(gameDir string) (string, error) {
	games, err := discovery.ScanRecursive(context.Background(), filepath.Dir(gameDir))
	if err != nil {
		return "", fmt.Errorf("resolve exe for %s: %w", gameDir, err)
	}
	want := canonicalDir(gameDir)
	for _, g := range games {
		if g.InstallDir == want && g.ExePath != "" {
			return g.ExePath, nil
		}
	}
	if exe, err := discovery.FindMainExe(context.Background(), gameDir); err == nil && exe != "" {
		return exe, nil
	}
	return "", fmt.Errorf("no executable found under %s", gameDir)
}

// disambiguateTitles appends a folder suffix to rows that share one
// title, so games whose exes carry identical metadata titles (both
// "TOI") stay distinguishable in the library. When the folder name is
// the title itself, the parent directory disambiguates ("Red Dead
// Redemption 2 (Games)" vs "(common)"); the full install dir is the
// last resort.
func disambiguateTitles(rows []GameRow) {
	squeeze := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '-' || r == '_' || r == '.' || r == ' ' {
				return -1
			}
			return unicode.ToLower(r)
		}, s)
	}
	groups := map[string][]int{}
	for i, r := range rows {
		groups[r.Title] = append(groups[r.Title], i)
	}
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		seen := map[string]bool{}
		for _, i := range idxs {
			suffix := filepath.Base(rows[i].InstallDir)
			if squeeze(suffix) == squeeze(rows[i].Title) {
				if parent := filepath.Base(filepath.Dir(rows[i].InstallDir)); parent != "" && squeeze(parent) != squeeze(rows[i].Title) {
					suffix = parent
				} else {
					suffix = rows[i].InstallDir
				}
			}
			for seen[suffix] {
				suffix = rows[i].InstallDir
			}
			seen[suffix] = true
			rows[i].Title += " (" + suffix + ")"
		}
	}
}

// canonicalDir mirrors the scanner's path canonicalization so install dirs
// compare equal across aliases and symlinks.
func canonicalDir(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Clean(p)
}

func (s *Session) doInstall(gameDir string, eacOK, cachedOK bool) {
	_ = s.runInstall(gameDir, eacOK, cachedOK)
}

// runInstall is doInstall's body with an outcome: nil on success,
// errInstallPaused when a consent gate owns the continuation, errOpBusy
// when the game already has an op, the context cause on cancel, and the
// surfaced error on failure. Callers that chain ops (upgrade) branch on
// the outcome; fire-and-forget callers discard it.
func (s *Session) runInstall(gameDir string, eacOK, cachedOK bool) error {
	row := s.findRow(gameDir)
	if row != nil && row.EAC && !eacOK {
		s.setConfirm(&Confirmation{
			Kind:    ConfirmEAC,
			GameDir: gameDir,
			Message: fmt.Sprintf("%s uses Easy Anti-Cheat. Installing OptiScaler may result in a ban.", row.Title),
		})
		return errInstallPaused
	}
	pre := preOpStatus(row)
	ctx, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return errOpBusy
	}
	s.opStarted("Installing…")
	// A scan that just resolved the default live leaves a provably fresh
	// release cache behind; serving it back without the stale-cache
	// consent prompt keeps scan-then-install a one-click flow.
	m, err := app.Install(ctx, s.deps.Store, s.deps.GH, s.deps.CacheDir, gameDir,
		app.InstallOpts{AllowCached: cachedOK || s.defaultRecentlyResolved(), EACOverride: eacOK, Requested: s.Settings().DefaultVersion})
	s.finishOp(gameDir)
	if errors.Is(err, context.Canceled) {
		s.opCancelled(gameDir, pre)
		return err
	}
	if errors.Is(err, app.ErrStaleCache) {
		s.opAborted()
		s.setConfirm(&Confirmation{
			Kind:    ConfirmCachedRelease,
			GameDir: gameDir,
			Message: "GitHub is rate-limiting; only stale cached release info is available. Use it anyway?",
		})
		return errInstallPaused
	}
	if err != nil {
		s.opFailed(err)
		return err
	}
	s.setRowInstalled(gameDir, m.Resolved.Version)
	s.opDone("Installed "+gameTitle(row, gameDir), gameDir)
	return nil
}

// errNotManagedToast is the user-facing refusal when an uninstall targets an
// install this manager never made (external, or its manifest vanished): the
// store holds no manifest, so no SHA-verified removal is possible and the
// raw store sentinel must never leak into a toast.
const errNotManagedToast = "not installed by this manager — adopt first or remove manually"

func (s *Session) doUninstall(gameDir string) {
	_ = s.runUninstall(gameDir)
}

// runUninstall is doUninstall's body with an outcome, mirroring
// runInstall: nil on success, app.ErrNotManaged on the refusal paths,
// errOpBusy when the game is busy, the context cause on cancel, and the
// surfaced error on failure.
func (s *Session) runUninstall(gameDir string) error {
	row := s.findRow(gameDir)
	if row != nil && row.Status == domain.StatusExternal {
		s.toast(errNotManagedToast, true)
		return app.ErrNotManaged
	}
	pre := preOpStatus(row)
	ctx, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return errOpBusy
	}
	s.opStarted("Uninstalling…")
	_, err := app.Uninstall(ctx, s.deps.Store, gameDir)
	s.finishOp(gameDir)
	if errors.Is(err, context.Canceled) {
		s.opCancelled(gameDir, pre)
		return err
	}
	if errors.Is(err, app.ErrNotManaged) {
		s.opRefused(errNotManagedToast, gameDir)
		return err
	}
	if err != nil {
		s.opFailed(err)
		return err
	}
	// Uninstall restores whatever the adopt-time backup held — possibly a
	// pre-existing external install. One bounded probe keeps the row honest:
	// external when a branded injection DLL is back, "" when the dir is clean.
	var injectionDir string
	if row != nil {
		injectionDir = row.InjectionDir
	}
	s.setRowStatus(gameDir, redetectExternal(injectionDir))
	s.opDone("Uninstalled "+gameTitle(s.findRow(gameDir), gameDir), gameDir)
	return nil
}

// redetectExternal runs the bounded post-op probe shared by uninstall and
// rollback: external when a branded injection DLL is (back) in injectionDir,
// "" otherwise. An empty injectionDir probes nothing.
func redetectExternal(injectionDir string) domain.Status {
	if injectionDir == "" {
		return ""
	}
	if found, _ := pever.DetectOptiScaler(injectionDir); found {
		return domain.StatusExternal
	}
	return ""
}

func preOpStatus(row *GameRow) domain.Status {
	if row != nil {
		return row.Status
	}
	return ""
}

// registerOp records a cancellable context for gameDir, serializing ops per
// game: false (and no context) when one is already in flight.
func (s *Session) registerOp(gameDir string) (context.Context, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.opCancels[gameDir]; busy {
		return nil, false
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.opCancels[gameDir] = cancel
	return ctx, true
}

// finishOp releases the op slot for gameDir. It runs before any terminal
// event is emitted so a follow-up op never sees a stale slot.
func (s *Session) finishOp(gameDir string) {
	s.mu.Lock()
	if cancel, ok := s.opCancels[gameDir]; ok {
		delete(s.opCancels, gameDir)
		cancel()
	}
	s.mu.Unlock()
}

// CancelOp cancels the in-flight install/uninstall/rollback for gameDir and
// reports whether one was running.
func (s *Session) CancelOp(gameDir string) bool {
	s.mu.Lock()
	cancel, ok := s.opCancels[gameDir]
	s.mu.Unlock()
	if ok {
		log.Info().Str("gameDir", gameDir).Msg("cancelling op")
		cancel()
	}
	return ok
}

// OpBusy reports whether gameDir has an op in flight. Frontends gate
// per-game cancel affordances on it (a global busy flag would point the
// button at the wrong game).
func (s *Session) OpBusy(gameDir string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, busy := s.opCancels[gameDir]
	return busy
}

func gameTitle(row *GameRow, dir string) string {
	if row != nil {
		return row.Title
	}
	return dir
}

// toRow enriches a library entry into display form, resolving cover art.
func (s *Session) toRow(ctx context.Context, e app.LibraryEntry) GameRow {
	row := GameRow{
		Title:             e.Game.Name,
		AppID:             e.Game.AppID,
		InstallDir:        e.Game.InstallDir,
		InjectionDir:      e.InjectionDir,
		Platform:          e.Game.Store.String(),
		Store:             e.Game.Store,
		AppName:           e.Game.AppName,
		ExePath:           e.Game.ExePath,
		CompatPrefix:      e.Game.CompatPrefix,
		OptiScalerVersion: e.OptiScalerVersion,
		Status:            e.Status,
		Actionable:        actionableStatus(e.Status),
		EAC:               e.EAC,
		ModTime:           e.ModTime,
		SteamAppID:        e.Game.SteamAppID,
		TitleSource:       string(e.Game.TitleSource),
	}
	row.UpgradeAvailable, row.UpgradeTarget = upgradeOffer(e.Status, e.OptiScalerVersion, s.resolvedDefault())
	keys := make([]string, 0, len(e.ComponentVersions))
	for k := range e.ComponentVersions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		row.Components = append(row.Components, e.ComponentVersions[k])
	}
	for _, tech := range e.Tech {
		row.TechBadges = append(row.TechBadges, badgeForTech(tech))
	}
	if s.deps.Covers != nil {
		coverAppID := e.Game.AppID
		if e.Game.SteamAppID != "" {
			coverAppID = e.Game.SteamAppID
		}
		if p, err := s.deps.Covers.Cover(ctx, coverAppID, e.Game.Name); err == nil {
			row.CoverPath = p
		}
	}
	return row
}

// refreshCovers rebinds cover art after the online identification phase
// has finalized titles and appids: rows with a resolved Steam appid get
// art for THAT appid (straight from the CDN), so a cover fetched for a
// codename title is replaced by the correct game's art the same scan.
func (s *Session) refreshCovers(ctx context.Context, rows []GameRow) {
	if s.deps.Covers == nil {
		return
	}
	for i := range rows {
		if rows[i].SteamAppID == "" {
			continue
		}
		if strings.HasSuffix(rows[i].CoverPath, rows[i].SteamAppID+".img") {
			continue
		}
		if p, err := s.deps.Covers.Cover(ctx, rows[i].SteamAppID, rows[i].Title); err == nil {
			rows[i].CoverPath = p
		}
	}
}

func (s *Session) findRow(dir string) *GameRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.st.Rows {
		if s.st.Rows[i].InstallDir == dir {
			row := s.st.Rows[i]
			return &row
		}
	}
	return nil
}

func (s *Session) setRowStatus(dir string, status domain.Status) {
	s.mu.Lock()
	changed := false
	for i := range s.st.Rows {
		if s.st.Rows[i].InstallDir == dir {
			s.st.Rows[i].Status = status
			s.st.Rows[i].Actionable = actionableStatus(status)
			// A row leaving the installed states can no longer honor an
			// upgrade offer computed against the old one — drop it rather
			// than let a stale offer dispatch a no-op upgrade. Committed
			// and external rows keep theirs (a restored external install
			// is exactly as old as it was when the offer was computed).
			if status != domain.StatusCommitted && status != domain.StatusExternal {
				s.st.Rows[i].UpgradeAvailable = false
				s.st.Rows[i].UpgradeTarget = ""
			}
			sortRows(s.st.Rows)
			changed = true
			break
		}
	}
	s.mu.Unlock()
	if changed {
		s.persistCache()
	}
}

// setRowInstalled settles a row after a successful install: committed at
// the just-installed version, with any upgrade offer consumed — a fresh
// install IS the resolved default, so offering an upgrade right after it
// would re-dispatch the chain on every quick click until the next scan.
func (s *Session) setRowInstalled(dir, version string) {
	s.mu.Lock()
	changed := false
	for i := range s.st.Rows {
		if s.st.Rows[i].InstallDir == dir {
			s.st.Rows[i].Status = domain.StatusCommitted
			s.st.Rows[i].Actionable = false
			s.st.Rows[i].OptiScalerVersion = version
			s.st.Rows[i].UpgradeAvailable = false
			s.st.Rows[i].UpgradeTarget = ""
			sortRows(s.st.Rows)
			changed = true
			break
		}
	}
	s.mu.Unlock()
	if changed {
		s.persistCache()
	}
}

// persistCache snapshots the current rows into the games cache (games.json
// in the data root). Serialized so concurrent scan/op writers cannot
// interleave; the last settled state wins.
func (s *Session) persistCache() {
	if s.deps.SettingsRoot == "" {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.mu.Lock()
	rows := append([]GameRow(nil), s.st.Rows...)
	s.mu.Unlock()
	saveGamesCache(s.deps.SettingsRoot, rows)
}

func (s *Session) setBusy(b string) {
	s.mu.Lock()
	s.st.Busy = b
	s.mu.Unlock()
}

func (s *Session) setStatus(line string) {
	s.mu.Lock()
	s.st.StatusLine = line
	s.mu.Unlock()
}

func (s *Session) setConfirm(c *Confirmation) {
	s.mu.Lock()
	s.st.Confirm = c
	s.mu.Unlock()
	s.emit(Event{Kind: EvConfirm, Text: c.Message, GameDir: c.GameDir})
}

func (s *Session) opStarted(what string) {
	s.setBusy(what)
	s.setStatus(what)
	s.emit(Event{Kind: EvOpStarted, Text: what})
}

func (s *Session) opDone(what, gameDir string) {
	s.setBusy("")
	s.setStatus(what)
	s.toast(what, false)
	s.emit(Event{Kind: EvOpDone, Text: what, GameDir: gameDir})
}

func (s *Session) opFailed(err error) {
	s.setBusy("")
	s.setStatus("Failed: " + err.Error())
	s.toast("Failed: "+err.Error(), true)
	s.emit(Event{Kind: EvOpFailed, Text: err.Error()})
}

// opRefused settles an op the store rejected as not manager-installed: busy
// clears and one clean warning toast/event surfaces — never the raw sentinel.
func (s *Session) opRefused(msg, gameDir string) {
	s.setBusy("")
	s.setStatus(msg)
	s.toast(msg, true)
	s.emit(Event{Kind: EvOpFailed, Text: msg, GameDir: gameDir})
}

// opAborted clears the busy state without a completion toast (used when an
// op pauses for consent instead of finishing).
func (s *Session) opAborted() {
	s.setBusy("")
	s.setStatus("Ready")
}

// opCancelled settles a cancelled op: the row returns to its pre-op status
// and exactly one "Cancelled" toast/event surfaces — never the failure path.
func (s *Session) opCancelled(gameDir string, pre domain.Status) {
	s.setBusy("")
	s.setStatus("Cancelled")
	s.setRowStatus(gameDir, pre)
	s.toast("Cancelled", false)
	s.emit(Event{Kind: EvOpCancelled, Text: "Cancelled", GameDir: gameDir})
}

func (s *Session) toast(text string, warn bool) {
	s.mu.Lock()
	s.st.Toasts = append(s.st.Toasts, Toast{Text: text, Warn: warn, AddedAt: s.now()})
	s.mu.Unlock()
}

func (s *Session) emit(ev Event) {
	select {
	case s.events <- ev:
	default:
		log.Warn().Int("kind", int(ev.Kind)).Msg("session event dropped (buffer full)")
	}
}
