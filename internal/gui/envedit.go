package gui

import (
	"strings"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Issue 067: the detail panel's per-game launch-env editor — an open
// dictionary of extra env vars (DXVK_*, PROTON_*, …) passed to the
// launched process. The title editor's discipline: explicit
// apply/cancel (not per-keystroke), Enter→apply / Esc→cancel carried
// by the editor's own editState, and the edit dies with the panel
// context it belongs to.

// startEnvEdit opens the inline env editor for row, pre-filled with
// the current pin in canonical form, and arms the deferred focus grab.
func (m *model) startEnvEdit(row ui.GameRow) {
	text := ui.FormatEnv(m.sess.UmuEnvFor(row.InstallDir))
	m.envEditDir = row.InstallDir
	m.envBuf = text
	m.envFocusPending = true
	m.envEditState = &editState{
		cursor:   len([]rune(text)),
		anchor:   -1,
		blink:    time.Now(),
		phase:    true,
		onEnter:  m.applyEnvEdit,
		onEscape: m.cancelEnvEdit,
	}
}

// applyEnvEdit commits the edited env through the session (empty
// clears the pin) and closes the editor. An invalid token keeps the
// editor open with the text intact — the toast names it.
func (m *model) applyEnvEdit() {
	dir := m.envEditDir
	text := strings.TrimSpace(m.envBuf)
	env, err := ui.ParseEnv(text)
	if err != nil {
		m.sess.Toast(err.Error(), true)
		return
	}
	m.closeEnvEdit()
	if m.sess == nil || dir == "" {
		return
	}
	m.sess.SetUmuEnvOverride(dir, env)
}

// cancelEnvEdit closes the env editor without applying.
func (m *model) cancelEnvEdit() {
	m.closeEnvEdit()
}

// closeEnvEdit resets every piece of env-editor state.
func (m *model) closeEnvEdit() {
	m.envEditDir = ""
	m.envBuf = ""
	m.envFocusPending = false
	m.envEditState = nil
}
