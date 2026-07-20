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

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/launch"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/pickdir"
	"github.com/cr1cr1/optiscaler-manager/internal/protondb"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/steam"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
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

	openExternal func(path string) error
	pickDir      func(ctx context.Context) (string, error)
	removeAll    func(path string) error
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

// openExternal opens path with the platform's default handler.
func openExternal(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	default: // linux and the rest
		return exec.Command("xdg-open", path).Start()
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
	rows := loadGamesCache(s.deps.SettingsRoot)
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

// Scan refreshes the library asynchronously.
func (s *Session) Scan(ctx context.Context) {
	go func() {
		s.emit(Event{Kind: EvScanStarted})
		s.setBusy("Scanning…")
		s.resetProgress()
		snap := s.Settings()
		entries, err := app.ScanAllLibraries(ctx, s.deps.Store, app.ScanAllOptions{
			SteamRoot: s.deps.SteamRoot,
			ExtraDirs: snap.ExtraDirs,
			Progress: func(phase string, done, total int) {
				if ctx.Err() != nil {
					return
				}
				s.scanProgress(phase, done, total)
			},
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
		coversTotal := len(entries) + len(snap.ExtraDirs)
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
		rows = s.mergeExtraDirs(ctx, rows, snap.ExtraDirs, coversTick)
		// Online lookup phase: enrich the local rows before they are
		// committed to state, so the final persistCache lands the enriched
		// fields in one write.
		s.enrichOnline(ctx, rows, snap)
		s.mu.Lock()
		// Directories added while this scan was in flight are not in the
		// snapshot's ExtraDirs; keep their rows rather than wiping them.
		fresh := map[string]bool{}
		for _, r := range rows {
			fresh[r.InstallDir] = true
		}
		for _, r := range s.st.Rows {
			if fresh[r.InstallDir] {
				continue
			}
			for _, d := range s.deps.Settings.ExtraDirs {
				if r.InstallDir == d {
					rows = append(rows, r)
					break
				}
			}
		}
		sortRows(rows)
		s.st.Rows = rows
		s.st.StatusLine = fmt.Sprintf("%d games", len(rows))
		s.st.Busy = ""
		s.st.Progress = nil
		s.mu.Unlock()
		s.persistCache()
		s.emit(Event{Kind: EvScanDone, Text: fmt.Sprintf("%d games", len(rows))})
	}()
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
// the one-click toggle from the reference client, with our defaults.
func (s *Session) QuickInstall(gameDir string) {
	row := s.findRow(gameDir)
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
	go func() {
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
		if err != nil {
			s.opFailed(err)
			return
		}
		s.setRowStatus(gameDir, domain.StatusRolledBack)
		s.opDone("Rolled back "+dir, gameDir)
	}()
}

// AddDirectory registers a user-picked directory as a game row and persists
// it in settings so later scans keep it. The call never blocks on
// enrichment: validation, settings persistence, and a placeholder row are
// synchronous (so a Scan started right after sees the directory), while the
// walk/classify/cover enrichment runs in a goroutine that replaces the
// placeholder and settles with the usual EvScanDone "directory added"
// event. A duplicate Add of the same canonical dir while one is in flight
// is rejected with a toast and no event.
func (s *Session) AddDirectory(dir string) {
	root, err := canonicalDirChecked(dir)
	if err != nil {
		s.toast("add directory: "+err.Error(), true)
		return
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
		entry, err := app.ManualEntry(dir)
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

// mergeExtraDirs appends rows for directories the user added manually.
// extraDirs must be a locked settings snapshot taken by the caller. tick,
// when non-nil, runs after each appended row (cover-progress accounting).
func (s *Session) mergeExtraDirs(ctx context.Context, rows []GameRow, extraDirs []string, tick func()) []GameRow {
	for _, d := range extraDirs {
		entry, err := app.ManualEntry(d)
		if err != nil {
			log.Warn().Err(err).Str("dir", d).Msg("extra dir unavailable")
			continue
		}
		dup := false
		for _, r := range rows {
			if r.InstallDir == entry.Game.InstallDir {
				dup = true
				break
			}
		}
		if !dup {
			rows = append(rows, s.toRow(ctx, entry))
			if tick != nil {
				tick()
			}
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

// OpenINI opens the game's OptiScaler.ini in the system editor.
func (s *Session) OpenINI(gameDir string) {
	row := s.findRow(gameDir)
	if row == nil || row.InjectionDir == "" {
		s.toast("no OptiScaler.ini (not installed?)", true)
		return
	}
	if err := s.openExternal(filepath.Join(row.InjectionDir, "OptiScaler.ini")); err != nil {
		s.toast("cannot open editor: "+err.Error(), true)
	}
}

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
// directory whose ExePath discovery left blank.
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
	return "", fmt.Errorf("no executable found under %s", gameDir)
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
	row := s.findRow(gameDir)
	if row != nil && row.EAC && !eacOK {
		s.setConfirm(&Confirmation{
			Kind:    ConfirmEAC,
			GameDir: gameDir,
			Message: fmt.Sprintf("%s uses Easy Anti-Cheat. Installing OptiScaler may result in a ban.", row.Title),
		})
		return
	}
	pre := preOpStatus(row)
	ctx, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return
	}
	s.opStarted("Installing…")
	_, err := app.Install(ctx, s.deps.Store, s.deps.GH, s.deps.CacheDir, gameDir,
		app.InstallOpts{AllowCached: cachedOK, EACOverride: eacOK, Requested: s.Settings().DefaultVersion})
	s.finishOp(gameDir)
	if errors.Is(err, context.Canceled) {
		s.opCancelled(gameDir, pre)
		return
	}
	if errors.Is(err, app.ErrStaleCache) {
		s.opAborted()
		s.setConfirm(&Confirmation{
			Kind:    ConfirmCachedRelease,
			GameDir: gameDir,
			Message: "GitHub is rate-limiting; only stale cached release info is available. Use it anyway?",
		})
		return
	}
	if err != nil {
		s.opFailed(err)
		return
	}
	s.setRowStatus(gameDir, domain.StatusCommitted)
	s.opDone("Installed "+gameTitle(row, gameDir), gameDir)
}

// errNotManagedToast is the user-facing refusal when an uninstall targets an
// install this manager never made (external, or its manifest vanished): the
// store holds no manifest, so no SHA-verified removal is possible and the
// raw store sentinel must never leak into a toast.
const errNotManagedToast = "not installed by this manager — adopt first or remove manually"

func (s *Session) doUninstall(gameDir string) {
	row := s.findRow(gameDir)
	if row != nil && row.Status == domain.StatusExternal {
		s.toast(errNotManagedToast, true)
		return
	}
	pre := preOpStatus(row)
	ctx, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return
	}
	s.opStarted("Uninstalling…")
	_, err := app.Uninstall(ctx, s.deps.Store, gameDir)
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
	// Uninstall restores whatever the adopt-time backup held — possibly a
	// pre-existing external install. One bounded probe keeps the row honest:
	// external when a branded injection DLL is back, "" when the dir is clean.
	status := domain.Status("")
	if row != nil && row.InjectionDir != "" {
		if found, _ := pever.DetectOptiScaler(row.InjectionDir); found {
			status = domain.StatusExternal
		}
	}
	s.setRowStatus(gameDir, status)
	s.opDone("Uninstalled "+gameTitle(s.findRow(gameDir), gameDir), gameDir)
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
	}
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
		if p, err := s.deps.Covers.Cover(ctx, e.Game.AppID, e.Game.Name); err == nil {
			row.CoverPath = p
		}
	}
	return row
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
