package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestGUIDetailEnvEditor: the detail panel's Env button opens the
// inline launch-env editor; Apply persists the pairs (invalid text
// keeps the editor open), Cancel closes without applying, and an
// empty Apply clears the pin (issue 067).
func TestGUIDetailEnvEditor(t *testing.T) {
	root := t.TempDir()
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.SettingsRoot = root
	})
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	headlessFrames(t, 1600, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.envBtnRect.Size[0] == 0 {
		t.Fatal("Env button not rendered (Linux detail panel must show it)")
	}

	clickRect(m.envBtnRect, m.rootView)
	if m.envEditDir != row.InstallDir {
		t.Fatalf("envEditDir = %q after Env click, want the selected game", m.envEditDir)
	}
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.envInputID == nil {
		t.Fatal("env input not rendered while editing")
	}

	// Invalid text: Apply keeps the editor open, nothing persists.
	m.envBuf = "DXVK_HUD"
	m.applyEnvEdit()
	if m.envEditDir == "" {
		t.Fatal("invalid env closed the editor — it must stay open")
	}
	if len(sess.Settings().EnvOverrides) != 0 {
		t.Errorf("invalid env persisted: %v", sess.Settings().EnvOverrides)
	}

	// Valid pairs: Apply persists and closes.
	m.envBuf = "DXVK_HUD=fps PROTON_ENABLE_NVAPI=1"
	m.applyEnvEdit()
	if m.envEditDir != "" {
		t.Fatal("valid env did not close the editor")
	}
	if got := sess.UmuEnvFor(row.InstallDir); got["DXVK_HUD"] != "fps" || got["PROTON_ENABLE_NVAPI"] != "1" {
		t.Fatalf("UmuEnvFor = %v, want the two pairs", got)
	}
	s, err := settings.Load(root)
	if err != nil {
		t.Fatalf("settings unreadable after apply: %v", err)
	}
	if len(s.EnvOverrides) != 1 {
		t.Fatalf("persisted EnvOverrides = %v, want exactly one pin", s.EnvOverrides)
	}

	// Empty Apply clears the pin.
	m.startEnvEdit(row)
	m.envBuf = ""
	m.applyEnvEdit()
	if len(sess.Settings().EnvOverrides) != 0 {
		t.Errorf("after empty apply EnvOverrides = %v, want empty", sess.Settings().EnvOverrides)
	}

	// Cancel closes without applying.
	m.startEnvEdit(row)
	m.envBuf = "SHOULD_NOT_APPLY=1"
	m.cancelEnvEdit()
	if m.envEditDir != "" {
		t.Error("cancel did not close the editor")
	}
	if len(sess.Settings().EnvOverrides) != 0 {
		t.Errorf("cancel applied anyway: %v", sess.Settings().EnvOverrides)
	}
}
