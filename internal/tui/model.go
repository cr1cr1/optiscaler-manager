// Package tui is the bubbletea frontend over the frontend-agnostic
// ui.Session: it renders session snapshots and forwards keypresses to
// session commands. It contains no business logic — install/uninstall/
// launch/rollback semantics live in internal/ui and below.
package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// screen identifies the active top-level screen (tab bar order).
type screen int

const (
	screenGames screen = iota
	screenDetail
	screenSettings
	screenHelp
	screenAbout
)

// inputMode identifies which text input is currently capturing keys.
type inputMode int

const (
	inputNone inputMode = iota
	inputFilter
	inputAddDir
	inputEditVersion
	inputEditTemplate
)

// Model is the bubbletea model bound to one ui.Session: one flat model with
// per-screen update/view handlers.
type Model struct {
	sess    *ui.Session
	version string // build version, rendered on the About screen

	screen       screen
	cursor       int // games row cursor
	dirCursor    int // settings directory cursor
	detailDir    string
	width        int
	height       int
	gamesVP      viewport.Model
	detailVP     viewport.Model
	settingsVP   viewport.Model
	input        textinput.Model
	mode         inputMode
	spin         spinner.Model
	confirmRmDir string // directory pending inline remove confirmation
}

// eventMsg carries one session event into the update loop.
type eventMsg ui.Event

// New builds the TUI model over sess; version is the build version shown on
// the About screen ("" renders as "dev").
func New(sess *ui.Session, version string) Model {
	ti := textinput.New()
	return Model{
		sess:    sess,
		version: version,
		input:   ti,
		spin:    spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
}

// Init boots the library cache-first (warm cache hydrates rows without a
// scan; a cold cache falls through to one) and subscribes to session events.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			m.sess.Start(context.Background())
			return nil
		},
		waitEvent(m.sess.Events()),
		m.spin.Tick,
	)
}

// waitEvent is the channel→Cmd bridge: one session event per tea.Msg,
// resubscribed after every event so the stream keeps flowing.
func waitEvent(events <-chan ui.Event) tea.Cmd {
	return func() tea.Msg {
		return eventMsg(<-events)
	}
}

// Update routes session events, terminal resizes, spinner ticks, and keys.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case eventMsg:
		_ = msg // the event is only a poke; View re-reads the snapshot
		m.clamp()
		return m, waitEvent(m.sess.Events())
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// clamp keeps both cursors inside their (possibly shrunken) lists.
func (m *Model) clamp() {
	if n := len(m.sess.VisibleRows()); n == 0 {
		m.cursor = 0
	} else if m.cursor >= n {
		m.cursor = n - 1
	}
	if n := len(m.sess.Settings().ExtraDirs); n == 0 {
		m.dirCursor = 0
	} else if m.dirCursor >= n {
		m.dirCursor = n - 1
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	snap := m.sess.Snapshot()

	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// A pending session confirmation is modal: only its answers are accepted.
	if snap.Confirm != nil {
		switch msg.String() {
		case "y", "Y":
			m.sess.AnswerConfirm(true)
		case "n", "N", "esc", "enter":
			m.sess.AnswerConfirm(false)
		}
		return m, nil
	}

	// The settings remove-directory confirmation is modal too.
	if m.confirmRmDir != "" {
		switch msg.String() {
		case "y", "Y":
			m.sess.RemoveDirectory(m.confirmRmDir)
			m.confirmRmDir = ""
		case "n", "N", "esc":
			m.confirmRmDir = ""
		}
		return m, nil
	}

	// An open text input captures all keys until committed or cancelled.
	if m.mode != inputNone {
		switch msg.Type {
		case tea.KeyEsc:
			m.cancelInput()
			return m, nil
		case tea.KeyEnter:
			m.commitInput()
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.mode == inputFilter {
			m.sess.SetQuery(m.input.Value())
			m.cursor = 0
		}
		return m, cmd
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "1":
		m.screen = screenGames
		return m, nil
	case "2":
		m.screen = screenSettings
		return m, nil
	case "3":
		m.screen = screenHelp
		return m, nil
	case "4":
		m.screen = screenAbout
		return m, nil
	}

	switch m.screen {
	case screenGames:
		return m.gamesKey(msg)
	case screenDetail:
		return m.detailKey(msg)
	case screenSettings:
		return m.settingsKey(msg)
	}
	return m, nil
}

func (m Model) gamesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.sess.VisibleRows()
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(rows)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if dir := selectedDir(rows, m.cursor); dir != "" {
			m.detailDir = dir
			m.screen = screenDetail
		}
	case "i":
		if dir := selectedDir(rows, m.cursor); dir != "" {
			m.sess.QuickInstall(dir)
		}
	case "l":
		if dir := selectedDir(rows, m.cursor); dir != "" {
			m.sess.Launch(dir)
		}
	case "c":
		if dir := selectedDir(rows, m.cursor); dir != "" {
			m.sess.CancelOp(dir)
		}
	case "/":
		m.openInput(inputFilter, "/ ", m.sess.Snapshot().Query)
	case "R":
		m.sess.Scan(context.Background())
	case "s":
		if m.sess.Snapshot().Sort == ui.SortName {
			m.sess.SetSort(ui.SortDefault)
		} else {
			m.sess.SetSort(ui.SortName)
		}
	}
	return m, nil
}

func (m Model) detailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	dir := m.detailDir
	switch msg.String() {
	case "esc", "enter":
		m.screen = screenGames
	case "i":
		m.sess.QuickInstall(dir)
	case "l":
		m.sess.Launch(dir)
	case "c":
		m.sess.CancelOp(dir)
	case "r":
		if row := m.detailRow(); row != nil && row.Actionable {
			m.sess.Rollback(dir)
		}
	case "o":
		if row := m.detailRow(); row != nil && row.Status == "committed" {
			m.sess.OpenINI(dir)
		}
	}
	return m, nil
}

func (m Model) settingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	dirs := m.sess.Settings().ExtraDirs
	switch msg.String() {
	case "j", "down":
		if m.dirCursor < len(dirs)-1 {
			m.dirCursor++
		}
	case "k", "up":
		if m.dirCursor > 0 {
			m.dirCursor--
		}
	case "e":
		m.openInput(inputEditVersion, "default version: ", m.sess.Settings().DefaultVersion)
	case "t":
		m.openInput(inputEditTemplate, "launch template: ", m.sess.Settings().LaunchTemplate)
	case "a":
		m.openInput(inputAddDir, "add dir: ", "")
	case "d":
		if m.dirCursor < len(dirs) {
			m.confirmRmDir = dirs[m.dirCursor]
		}
	case "o":
		m.sess.SetOnlineLookups(!m.sess.Settings().OnlineLookups)
	case "x":
		m.sess.ClearBundleCache()
	}
	return m, nil
}

// detailRow re-reads the selected game's row from a single snapshot.
func (m Model) detailRow() *ui.GameRow {
	rows := m.sess.Snapshot().Rows
	for i := range rows {
		if rows[i].InstallDir == m.detailDir {
			row := rows[i]
			return &row
		}
	}
	return nil
}

func (m *Model) openInput(mode inputMode, prompt, initial string) {
	m.mode = mode
	m.input.Prompt = prompt
	m.input.SetValue(initial)
	m.input.Focus()
}

func (m *Model) cancelInput() {
	if m.mode == inputFilter {
		m.sess.SetQuery("")
	}
	m.mode = inputNone
	m.input.SetValue("")
	m.input.Blur()
}

func (m *Model) commitInput() {
	v := strings.TrimSpace(m.input.Value())
	switch m.mode {
	case inputFilter:
		// the query already narrowed live; Enter only closes the input
	case inputAddDir:
		if v != "" {
			m.sess.AddDirectory(v) // invalid paths toast through the session
		}
	case inputEditVersion:
		m.sess.SetDefaultVersion(v)
	case inputEditTemplate:
		m.sess.SetLaunchTemplate(v)
	}
	m.mode = inputNone
	m.input.SetValue("")
	m.input.Blur()
}

func selectedDir(rows []ui.GameRow, cursor int) string {
	if cursor < 0 || cursor >= len(rows) {
		return ""
	}
	return rows[cursor].InstallDir
}
