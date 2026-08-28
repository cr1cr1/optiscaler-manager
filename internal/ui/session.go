package ui

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/launch"
	"github.com/cr1cr1/optiscaler-manager/internal/pcgw"
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
	// ConfirmVersionSwitch pauses a per-game version switch BEFORE any
	// destructive step (ini capture/removal, uninstall): Version carries
	// the chosen tag, and accepting re-enters the full switch chain with
	// EAC consent already granted — a mid-switch pause used to strand the
	// game uninstalled with the ini already deleted.
	ConfirmVersionSwitch
)

// Confirmation is a pending consent request. Installs never proceed past
// these points until AnswerConfirm(true) — the frontend renders the prompt.
type Confirmation struct {
	Kind    ConfirmKind
	GameDir string
	Message string
	// Version pins the release the paused install was started with (""
	// for the configured default): a version-switched install paused at a
	// consent gate resumes at the SAME tag, not whatever the default
	// happens to resolve to when the answer lands.
	Version string
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

	// UmuLauncher, when non-nil, is invoked for umu-eligible manual-store
	// games (Linux + Windows binary + UmuEnabled setting). It bypasses
	// the regular Launcher entirely. Construction typically wraps
	// umu.Detect + umu.Launch; nil on non-Linux or when umu-run is not
	// on PATH, in which case umu-eligible games fall through to the
	// regular Launcher (which usually fails on a Windows binary without
	// Proton, but the failure is honest).
	UmuLauncher UmuLauncherHook

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

// UmuLauncherHook launches a manual-store Windows binary via umu-run.
// Returning nil means the launch was requested; a non-nil error is
// surfaced as a launch failure (EvOpFailed + warn toast). The hook
// must NOT fall back to the regular Launcher — that's the caller's job
// when the hook itself is nil.
type UmuLauncherHook func(ctx context.Context, row GameRow) error

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
	// uninstall and install legs of a committed-row version switch (see
	// doSwitchVersion); nil in production. It lets a test occupy the
	// game's op slot in the finishOp→registerOp gap so the install leg's
	// errOpBusy path is exercised deterministically.
	upgradeGapHook func(gameDir string)

	// switchINIHook is a test seam invoked synchronously after a version
	// switch's install leg succeeded, just before the captured
	// OptiScaler.ini is written back (see doSwitchVersion); nil in
	// production. It lets a test sabotage the write-back (e.g. turn the
	// ini path into a directory) deterministically.
	switchINIHook func(gameDir string)

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

// setRowInstalled settles a row after a successful install: committed at
// the just-installed version.
func (s *Session) setRowInstalled(dir, version string) {
	s.mu.Lock()
	changed := false
	for i := range s.st.Rows {
		if s.st.Rows[i].InstallDir == dir {
			s.st.Rows[i].Status = domain.StatusCommitted
			s.st.Rows[i].Actionable = false
			s.st.Rows[i].OptiScalerVersion = version
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
