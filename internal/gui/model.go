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
	cfg          Config
	sess         *ui.Session
	state        ui.State
	filter       string
	auditGrid    bool
	about        bool
	settingsOpen bool
	versionBuf   string
	cols         int            // current grid columns, derived from live width
	cardW        int            // current card width in px, derived from live width
	cardH        int            // current card height in px
	exitNow      func(code int) // quit seam: os.Exit in production, stubbed in tests
}

func newModel(cfg Config) *model {
	return &model{
		cfg:       cfg,
		sess:      cfg.Session,
		auditGrid: cfg.AuditGrid,
		cols:      4,
		cardW:     190,
		cardH:     310,
		state:     ui.State{Mode: ui.ViewGrid, StatusLine: "Ready"},
		exitNow:   os.Exit,
	}
}

// Run opens the window and drives the frame loop.
func Run(ctx context.Context, cfg Config) error {
	shireiapp.SetupWindow("optiscaler-manager", 1100, 700)
	m := newModel(cfg)
	if m.sess != nil {
		m.sess.Scan(ctx)
	}
	shireiapp.Run(m.rootView)
	return nil
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

// exit flushes a pending settings-modal edit through the session's
// persistence path, then quits via the injected seam (shirei has no
// app.Quit, so production exits the process).
func (m *model) exit() {
	if m.sess != nil && m.versionBuf != "" && m.versionBuf != m.sess.Settings().DefaultVersion {
		m.sess.SetDefaultVersion(m.versionBuf)
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
