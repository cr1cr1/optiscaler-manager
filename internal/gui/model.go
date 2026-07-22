// Package gui is the go-shirei frontend: a thin binding over ui.Session.
// All interaction logic lives in internal/ui; this package only renders
// session state and forwards commands.
package gui

import (
	"context"
	"os"

	shireiapp "go.hasen.dev/shirei/app"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// renderToPNG is the headless render seam used by smoke tests.
var renderToPNG = RenderToPNG

// Config is what a frontend launch needs: the session to bind to.
type Config struct {
	Session   *ui.Session
	AuditGrid bool
	Version   string
}

// model is the shirei-side binding state: the latest session snapshot plus
// widget-local buffers.
type model struct {
	cfg                 Config
	sess                *ui.Session
	ctx                 context.Context // boot/launch context; Background in tests
	state               ui.State
	filter              string
	auditGrid           bool
	about               bool
	settingsOpen        bool
	versionBuf          string
	templateBuf         string
	onlineBuf           bool                          // settings-modal online-lookups toggle buffer, primed on open
	selIdx              int                           // keyboard-driven selection index into visible rows
	hoveredDir          string                        // install dir of the card under the mouse, "" when none
	cardRect            Rect                          // screen rect of the last rendered card (hover test seam)
	cardBtnRect         Rect                          // screen rect of the card's first button (click routing test seam)
	sidebarRects        []Rect                        // screen rects of the sidebar nav items (uniformity test seam)
	sidebarShellRect    Rect                          // screen rect of the sidebar shell (full-height test seam)
	progressTrackRect   Rect                          // screen rect of the scan progress track (progress bar test seam)
	progressFillRect    Rect                          // screen rect of the scan progress fill (progress bar test seam)
	tierPillRect        Rect                          // screen rect of the card's ProtonDB tier pill (tier badge test seam)
	detailPanelRect     Rect                          // screen rect of the detail panel shell (panel width test seam)
	listSegRect         Rect                          // screen rect of the List view-switch segment (click test seam)
	listSelRect         Rect                          // screen rect of the keyboard-selected list row (nav test seam)
	listRowRects        []Rect                        // screen rects of rendered list rows by row index, rebuilt each list frame (row-click test seam)
	listSelectedRect    Rect                          // screen rect of the session-selected list row's selection band (selected-highlight test seam)
	openINIRect         Rect                          // screen rect of the detail panel's OpenINI button (visibility test seam)
	searchID            ContainerId                   // the search field's container (`/` focuses it from anywhere)
	listID              ContainerId                   // the list view's focusable wrapper (Tab focus nav test seam)
	listFocusPending    bool                          // deferred row-click focus grab: consumed once by actionList with the wrapper's fresh identity
	listFocusRing       bool                          // whether the list wrapper drew its focus ring on the last frame (focus ring test seam)
	gridID              ContainerId                   // the grid view's focusable wrapper (Tab focus nav test seam, mirrors listID)
	gridFocusPending    bool                          // deferred card-click focus grab: consumed once by gridView with the wrapper's fresh identity (mirrors listFocusPending)
	gridFocusRing       bool                          // whether the grid wrapper drew its focus ring on the last frame (focus ring test seam, mirrors listFocusRing)
	gridCursorRect      Rect                          // screen rect of the keyboard-cursor grid card (cursor chrome test seam, mirrors listSelRect)
	cols                int                           // current grid columns, derived from live width
	cardW               int                           // current card width in px, derived from live width
	cardH               int                           // current card height in px
	exitNow             func(code int)                // quit seam: os.Exit in production, stubbed in tests
	switchVersionFn     func(gameDir, version string) // version-switch dispatch seam: nil in production (Session.SwitchVersion), stubbed in tests
	versionDDRects      map[string]Rect               // screen rects of rendered version-dropdown triggers by install dir (dropdown test seam)
	versionDDItems      []versionDDItem               // rows of the currently open version dropdown (dropdown test seam)
	versionDDItemsFor   string                        // install dir owning versionDDItems ("" = no dropdown open)
	openDropdownDir     string                        // install dir of the single open version dropdown ("" = none)
	ddTriggerID         ContainerId                   // container id of the card's version-dropdown trigger (click routing seam: the card must not steal its activation)
	ddFocusRing         bool                          // whether the version-dropdown trigger drew its focus ring on the last frame (keyboard focus test seam, mirrors listFocusRing)
	sortTriggerID       ContainerId                   // container id of the toolbar sort-dropdown trigger (focus/click test seam, mirrors ddTriggerID)
	sortFocusRing       bool                          // whether the sort trigger drew its focus ring on the last frame (keyboard focus test seam, mirrors ddFocusRing)
	sortMenuItems       []sortMenuItem                // rows of the currently open sort dropdown, rebuilt each popup frame and emptied while closed (sort dropdown test seam, mirrors versionDDItems)
	viewSwitchID        ContainerId                   // container id of the view switch's outer wrapper (focus/click test seam, mirrors sortTriggerID)
	viewSwitchFocusRing bool                          // whether the view-switch wrapper drew its focus ring on the last frame (keyboard focus test seam, mirrors sortFocusRing)
}

func newModel(cfg Config) *model {
	return &model{
		cfg:       cfg,
		sess:      cfg.Session,
		ctx:       context.Background(),
		auditGrid: cfg.AuditGrid,
		cols:      4,
		cardW:     190,
		cardH:     300,
		state:     ui.State{Mode: ui.ViewGrid, StatusLine: "Ready"},
		exitNow:   os.Exit,
	}
}

// Run opens the window and drives the frame loop.
func Run(ctx context.Context, cfg Config) error {
	shireiapp.SetupWindow("optiscaler-manager", 1100, 700)
	m := newModel(cfg)
	m.ctx = ctx
	m.boot(ctx)
	shireiapp.Run(m.rootView)
	return nil
}

// boot kicks off the session's cache-first startup: a warm games cache shows
// rows instantly; a cold cache falls through to a full scan inside Start.
func (m *model) boot(ctx context.Context) {
	if m.sess != nil {
		m.sess.Start(ctx)
	}
}

// drain pulls pending session events and refreshes the local snapshot.
func (m *model) drain() {
	if m.sess == nil {
		return
	}
	for {
		select {
		case <-m.sess.Events():
		default:
			m.state = m.sess.Snapshot()
			return
		}
	}
}

// syncFilter pushes the text buffer into the session when it changed.
func (m *model) syncFilter() {
	if m.sess == nil || m.filter == m.state.Query {
		return
	}
	m.sess.SetQuery(m.filter)
}

// visibleRows is the render row set: session-filtered when bound.
func (m *model) visibleRows() []ui.GameRow {
	if m.sess != nil {
		return m.sess.VisibleRows()
	}
	return m.state.Rows
}

// selectedRow resolves the dashboard target in the current snapshot.
func (m *model) selectedRow() *ui.GameRow {
	for i := range m.state.Rows {
		if m.state.Rows[i].InstallDir == m.state.Selected {
			return &m.state.Rows[i]
		}
	}
	return nil
}

// libraryEmpty reports whether the library has no rows at all; toolbar
// controls that only make sense with games are disabled while it is true.
func (m *model) libraryEmpty() bool {
	return len(m.state.Rows) == 0
}

// setSort forwards the toolbar sort choice to the session.
func (m *model) setSort(mode ui.SortMode) {
	if m.sess != nil {
		m.sess.SetSort(mode)
	}
}

// settingsDirs is the settings-modal directory list: the session's ExtraDirs.
func (m *model) settingsDirs() []string {
	if m.sess == nil {
		return nil
	}
	return m.sess.Settings().ExtraDirs
}

// applySettings persists the settings-modal buffers through the session.
func (m *model) applySettings() {
	if m.sess == nil {
		return
	}
	m.sess.SetDefaultVersion(m.versionBuf)
	m.sess.SetLaunchTemplate(m.templateBuf)
}

// exit flushes a pending settings-modal edit through the session's
// persistence path, then quits via the injected seam (shirei has no
// app.Quit, so production exits the process).
func (m *model) exit() {
	if m.sess != nil {
		cur := m.sess.Settings()
		if m.versionBuf != "" && m.versionBuf != cur.DefaultVersion {
			m.sess.SetDefaultVersion(m.versionBuf)
		}
		if m.templateBuf != "" && m.templateBuf != cur.LaunchTemplate {
			m.sess.SetLaunchTemplate(m.templateBuf)
		}
	}
	m.exitNow(0)
}

// launchGame starts the game when the row carries launch identity.
func (m *model) launchGame(e ui.GameRow) {
	if m.sess == nil || !launchable(&e) {
		return
	}
	m.sess.Launch(e.InstallDir)
}

// dispatchSwitchVersion forwards a version-dropdown pick to the session's
// async switcher. Tests inject switchVersionFn to capture the dispatch
// without running the real uninstall/install chain (mirrors the exitNow
// seam); production leaves it nil and goes straight to the session.
func (m *model) dispatchSwitchVersion(gameDir, version string) {
	if m.switchVersionFn != nil {
		m.switchVersionFn(gameDir, version)
		return
	}
	if m.sess != nil {
		m.sess.SwitchVersion(gameDir, version)
	}
}
