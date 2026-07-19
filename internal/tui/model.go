// Package tui is the bubbletea frontend over the frontend-agnostic
// ui.Session: it renders session snapshots and forwards keypresses to
// session commands. It contains no business logic — install/uninstall/
// launch/rollback semantics live in internal/ui and below.
package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Model is the bubbletea model bound to one ui.Session.
type Model struct {
	sess   *ui.Session
	cursor int  // index into the session's visible rows
	filter bool // '/' filter input mode active
	help   bool // '?'/f1 help line visible
}

// eventMsg carries one session event into the update loop.
type eventMsg ui.Event

// New builds the TUI model over sess.
func New(sess *ui.Session) Model {
	return Model{sess: sess}
}

// Init kicks the initial library scan and subscribes to session events.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			m.sess.Scan(context.Background())
			return nil
		},
		waitEvent(m.sess.Events()),
	)
}

// waitEvent is the channel→Cmd bridge: one session event per tea.Msg,
// resubscribed after every event so the stream keeps flowing.
func waitEvent(events <-chan ui.Event) tea.Cmd {
	return func() tea.Msg {
		return eventMsg(<-events)
	}
}

// Update routes session events and keypresses.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case eventMsg:
		_ = msg // the event is only a poke; View re-reads the snapshot
		if n := len(m.sess.VisibleRows()); n > 0 && m.cursor >= n {
			m.cursor = n - 1
		}
		return m, waitEvent(m.sess.Events())
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	snap := m.sess.Snapshot()

	// A pending confirmation is modal: only its answers are accepted.
	if snap.Confirm != nil {
		switch msg.String() {
		case "y", "Y":
			m.sess.AnswerConfirm(true)
		case "n", "N", "esc", "enter":
			m.sess.AnswerConfirm(false)
		}
		return m, nil
	}

	if m.filter {
		switch msg.Type {
		case tea.KeyEsc:
			m.filter = false
			m.sess.SetQuery("")
		case tea.KeyEnter:
			m.filter = false
		case tea.KeyBackspace:
			if q := []rune(snap.Query); len(q) > 0 {
				m.sess.SetQuery(string(q[:len(q)-1]))
			}
		case tea.KeyRunes:
			m.sess.SetQuery(snap.Query + string(msg.Runes))
		}
		m.cursor = 0
		return m, nil
	}

	rows := m.sess.VisibleRows()
	switch msg.String() {
	case "q":
		return m, tea.Quit
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
		m.filter = true
	case "?", "f1":
		m.help = !m.help
	}
	return m, nil
}

func selectedDir(rows []ui.GameRow, cursor int) string {
	if cursor < 0 || cursor >= len(rows) {
		return ""
	}
	return rows[cursor].InstallDir
}

// View renders the current session snapshot.
func (m Model) View() string {
	snap := m.sess.Snapshot()
	rows := m.sess.VisibleRows()
	var b strings.Builder

	b.WriteString("OptiScaler Manager — TUI\n")
	switch {
	case m.filter:
		fmt.Fprintf(&b, "/%s\n", snap.Query)
	case snap.Query != "":
		fmt.Fprintf(&b, "filter: %s\n", snap.Query)
	}
	b.WriteString("\n")

	if len(rows) == 0 {
		b.WriteString("(no matches)\n")
	}
	for i, r := range rows {
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		fmt.Fprintf(&b, "%s%s\n", marker, rowLine(r))
	}

	if snap.Confirm != nil {
		b.WriteString("\n")
		fmt.Fprintf(&b, "! %s\n", snap.Confirm.Message)
		b.WriteString("  proceed? [y/N]\n")
	}

	b.WriteString("\n")
	if m.help {
		b.WriteString("j/k move · enter install/uninstall · l launch · c cancel · / filter · ? help · q quit\n")
	}

	status := snap.StatusLine
	if snap.Busy != "" {
		status = snap.Busy
	}
	fmt.Fprintf(&b, "%s\n", status)
	if n := len(snap.Toasts); n > 0 {
		fmt.Fprintf(&b, "%s\n", snap.Toasts[n-1].Text)
	}
	return b.String()
}

// rowLine renders one game row as plain text: title, store, tech badges,
// component/OptiScaler versions, and install status.
func rowLine(r ui.GameRow) string {
	var b strings.Builder
	b.WriteString(r.Title)
	fmt.Fprintf(&b, "  %s", r.Platform)
	for _, badge := range r.TechBadges {
		fmt.Fprintf(&b, " [%s]", badge.Label)
	}
	versions := append([]string(nil), r.Components...)
	if r.OptiScalerVersion != "" {
		versions = append([]string{"OptiScaler " + r.OptiScalerVersion}, versions...)
	}
	if len(versions) > 0 {
		fmt.Fprintf(&b, "  %s", strings.Join(versions, ", "))
	}
	status := string(r.Status)
	if status == "" {
		status = "not installed"
	}
	fmt.Fprintf(&b, "  %s", status)
	return b.String()
}
