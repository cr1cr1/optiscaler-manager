package ui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
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
	EvConfirm
)

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
	Selected   string
	Busy       string // op description, "" when idle
	StatusLine string
	Confirm    *Confirmation
	Toasts     []Toast
}

// Deps wires the session to the lower layers.
type Deps struct {
	Store     *store.Store
	GH        *gh.Client
	Covers    *covers.Covers
	CacheDir  string
	SteamRoot string
}

// Session is the frontend-agnostic interactive core.
type Session struct {
	deps   Deps
	events chan Event

	mu  sync.Mutex
	st  State
	now func() time.Time

	openExternal func(path string) error
}

// NewSession starts a session. The library is empty until Scan is called.
func NewSession(deps Deps) *Session {
	return &Session{
		deps:   deps,
		events: make(chan Event, 64),
		st:     State{Mode: ViewGrid, StatusLine: "Ready"},
		now:    time.Now,
		openExternal: func(path string) error {
			return exec.Command("xdg-open", path).Start()
		},
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

// VisibleRows returns the rows after the current query filter.
func (s *Session) VisibleRows() []GameRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return filterRows(append([]GameRow(nil), s.st.Rows...), s.st.Query)
}

// SetQuery updates the search filter (cheap, synchronous).
func (s *Session) SetQuery(q string) {
	s.mu.Lock()
	s.st.Query = q
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

// Scan refreshes the library asynchronously.
func (s *Session) Scan(ctx context.Context) {
	go func() {
		s.emit(Event{Kind: EvScanStarted})
		s.setBusy("Scanning…")
		entries, err := app.ScanLibrary(ctx, s.deps.Store, s.deps.SteamRoot)
		if err != nil {
			s.setBusy("")
			s.setStatus("Scan failed: " + err.Error())
			log.Warn().Err(err).Msg("scan failed")
			s.emit(Event{Kind: EvScanFailed, Text: err.Error()})
			return
		}
		rows := make([]GameRow, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, s.toRow(ctx, e))
		}
		sortRows(rows)
		s.mu.Lock()
		s.st.Rows = rows
		s.st.StatusLine = fmt.Sprintf("%d games", len(rows))
		s.st.Busy = ""
		s.mu.Unlock()
		s.emit(Event{Kind: EvScanDone, Text: fmt.Sprintf("%d games", len(rows))})
	}()
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
		s.opStarted("Rolling back…")
		dir, err := app.Rollback(context.Background(), s.deps.Store, gameDir)
		if err != nil {
			s.opFailed(err)
			return
		}
		s.setRowStatus(gameDir, domain.StatusRolledBack)
		s.opDone("Rolled back "+dir, gameDir)
	}()
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
	s.opStarted("Installing…")
	_, err := app.Install(context.Background(), s.deps.Store, s.deps.GH, s.deps.CacheDir, gameDir,
		app.InstallOpts{AllowCached: cachedOK, EACOverride: eacOK})
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

func (s *Session) doUninstall(gameDir string) {
	s.opStarted("Uninstalling…")
	_, err := app.Uninstall(context.Background(), s.deps.Store, gameDir)
	if err != nil {
		s.opFailed(err)
		return
	}
	s.setRowStatus(gameDir, "")
	s.opDone("Uninstalled "+gameTitle(s.findRow(gameDir), gameDir), gameDir)
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
		Title:        e.Game.Name,
		AppID:        e.Game.AppID,
		InstallDir:   e.Game.InstallDir,
		InjectionDir: e.InjectionDir,
		Status:       e.Status,
		Actionable:   actionableStatus(e.Status),
		EAC:          e.EAC,
		ModTime:      e.ModTime,
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
	defer s.mu.Unlock()
	for i := range s.st.Rows {
		if s.st.Rows[i].InstallDir == dir {
			s.st.Rows[i].Status = status
			s.st.Rows[i].Actionable = actionableStatus(status)
			sortRows(s.st.Rows)
			return
		}
	}
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

// opAborted clears the busy state without a completion toast (used when an
// op pauses for consent instead of finishing).
func (s *Session) opAborted() {
	s.setBusy("")
	s.setStatus("Ready")
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
