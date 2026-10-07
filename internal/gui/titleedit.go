package gui

import (
	"strings"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Issue 028: the detail panel's manual identification fixes — pinning a
// display title and user-uploaded poster art. The title edit is an
// explicit apply/cancel flow (not the settings modal's live-diff apply):
// every applied change persists settings and kicks a cover re-resolution,
// so per-keystroke commits would spam both.

// startTitleEdit opens the inline title editor for row, pre-filled with
// its current title.
func (m *model) startTitleEdit(row ui.GameRow) {
	m.titleEditDir = row.InstallDir
	m.titleBuf = row.Title
}

// applyTitleEdit commits the edited title through the session (empty
// clears the override) and closes the editor.
func (m *model) applyTitleEdit() {
	dir := m.titleEditDir
	title := strings.TrimSpace(m.titleBuf)
	m.titleEditDir = ""
	m.titleBuf = ""
	if m.sess == nil || dir == "" {
		return
	}
	m.sess.SetTitleOverride(dir, title)
}

// cancelTitleEdit closes the title editor without applying.
func (m *model) cancelTitleEdit() {
	m.titleEditDir = ""
	m.titleBuf = ""
}
