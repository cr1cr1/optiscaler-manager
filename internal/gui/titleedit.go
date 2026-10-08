package gui

import (
	"strings"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Issue 028: the detail panel's manual identification fixes — pinning a
// display title and user-uploaded poster art. The title edit is an
// explicit apply/cancel flow (not the settings modal's live-diff apply):
// every applied change persists settings and kicks a cover re-resolution,
// so per-keystroke commits would spam both. Issue 031: the editor's own
// edit state carries Enter→apply / Esc→cancel hooks (editKeys consumes
// both keys by default, so a post-render check never sees them).

// startTitleEdit opens the inline title editor for row, pre-filled with
// its current title, and arms the deferred focus grab: the input does not
// exist until the header renders it next frame, so the focus lands there
// (issue 032; the cardFocusPending idiom).
func (m *model) startTitleEdit(row ui.GameRow) {
	m.titleEditDir = row.InstallDir
	m.titleBuf = row.Title
	m.titleEditOrig = row.Title
	m.titleFocusPending = true
	m.titleEditState = &editState{
		cursor:   len([]rune(row.Title)),
		anchor:   -1,
		blink:    time.Now(),
		phase:    true,
		onEnter:  m.applyTitleEdit,
		onEscape: m.cancelTitleEdit,
	}
}

// applyTitleEdit commits the edited title through the session (empty
// clears the override) and closes the editor. An unchanged title is never
// written (issue 032): a no-op commit would still persist settings and
// kick a cover re-resolution for nothing.
func (m *model) applyTitleEdit() {
	dir := m.titleEditDir
	title := strings.TrimSpace(m.titleBuf)
	unchanged := title == m.titleEditOrig
	m.closeTitleEdit()
	if m.sess == nil || dir == "" || unchanged {
		return
	}
	m.sess.SetTitleOverride(dir, title)
}

// cancelTitleEdit closes the title editor without applying.
func (m *model) cancelTitleEdit() {
	m.closeTitleEdit()
}

// closeTitleEdit resets every piece of title-editor state.
func (m *model) closeTitleEdit() {
	m.titleEditDir = ""
	m.titleBuf = ""
	m.titleEditOrig = ""
	m.titleFocusPending = false
	m.titleEditState = nil
}
