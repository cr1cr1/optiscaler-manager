package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestDetailKeySetEnv: 'E' on the detail screen opens the launch-env
// input pre-filled with the current pin; committing KEY=VALUE pairs
// persists them, an invalid token toasts and keeps the text, and an
// empty commit clears (issue 067).
func TestDetailKeySetEnv(t *testing.T) {
	settingsDir := t.TempDir()
	e := newTestEnv(t, func(d *ui.Deps) { d.SettingsRoot = settingsDir })
	seedGamesCache(t, settingsDir, []ui.GameRow{{
		Title: "EnvGame", AppID: "manual_env",
		InstallDir: e.gameRoot, Platform: "manual",
	}})
	e.sess.Start(context.Background())
	pollUntil(t, "cached row to load", func() bool {
		return findRow(e.sess.Snapshot().Rows, e.gameRoot) != nil
	})

	m := New(e.sess, "v0.0.0-test")
	m.screen = screenDetail
	m.detailDir = e.gameRoot

	mi, _ := m.detailKey(runeKey('E'))
	m = mi.(Model)
	if m.mode != inputSetEnv {
		t.Fatalf("mode = %v after 'E', want inputSetEnv", m.mode)
	}
	if m.input.Value() != "" {
		t.Errorf("input pre-fill = %q, want empty (no pin yet)", m.input.Value())
	}

	// Invalid token: toast + the input reopens with the text intact.
	m.input.SetValue("DXVK_HUD")
	m.commitInput()
	if m.mode != inputSetEnv {
		t.Fatalf("mode = %v after invalid commit, want inputSetEnv (kept open)", m.mode)
	}
	if m.input.Value() != "DXVK_HUD" {
		t.Errorf("input after invalid commit = %q, want the bad text kept", m.input.Value())
	}
	if len(e.sess.Settings().EnvOverrides) != 0 {
		t.Errorf("invalid commit persisted: %v", e.sess.Settings().EnvOverrides)
	}

	// Valid pairs persist.
	m.input.SetValue("DXVK_HUD=fps PROTON_ENABLE_NVAPI=1")
	m.commitInput()
	pollUntil(t, "env pin persisted", func() bool {
		s, err := settings.Load(settingsDir)
		if err != nil {
			return false
		}
		for _, pin := range s.EnvOverrides {
			return pin["DXVK_HUD"] == "fps" && pin["PROTON_ENABLE_NVAPI"] == "1"
		}
		return false
	})

	// Reopen: the pin is pre-filled in canonical form; empty clears.
	mi, _ = m.detailKey(runeKey('E'))
	m = mi.(Model)
	if got := m.input.Value(); !strings.Contains(got, "DXVK_HUD=fps") {
		t.Errorf("reopen pre-fill = %q, want it containing DXVK_HUD=fps", got)
	}
	m.input.SetValue("")
	m.commitInput()
	pollUntil(t, "env pin cleared", func() bool {
		return len(e.sess.Settings().EnvOverrides) == 0
	})
}
