package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
	"github.com/cr1cr1/optiscaler-manager/internal/umu"
)

var guiTestRunners = []umu.Runner{
	{Name: "UMU-Proton-10.0-4", Path: "/runners/UMU-Proton-10.0-4", Source: "steam"},
	{Name: "soda-9.0-1", Path: "/runners/soda-9.0-1", Source: "bottles"},
}

// TestGUISettingsUmuRunner: the global umu runner is a dropdown of
// Auto + the detected Proton builds ("Launcher: runner"); a pick
// buffers and applySettings persists (issue 063).
func TestGUISettingsUmuRunner(t *testing.T) {
	root := t.TempDir()
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.SettingsRoot = root
		d.Settings = settings.Defaults()
		d.UmuRunners = func() []umu.Runner { return guiTestRunners }
	})
	m := newModel(Config{Session: sess})
	m.openSettings()

	headlessFrames(t, 1000, 700)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.umuRunnerTriggerRect.Size[0] == 0 {
		t.Fatal("runner trigger not rendered in settings")
	}

	clickRect(m.umuRunnerTriggerRect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	items := m.umuRunnerMenuItems
	if len(items) != 3 {
		t.Fatalf("runner rows = %d, want 3: %+v", len(items), items)
	}
	wantLabels := []string{"Auto (newest installed)", "Steam: UMU-Proton-10.0-4", "Bottles: soda-9.0-1"}
	for i, w := range wantLabels {
		if items[i].label != w {
			t.Errorf("items[%d].label = %q, want %q", i, items[i].label, w)
		}
	}

	clickRect(items[2].rect, m.rootView) // Bottles: soda-9.0-1
	if m.umuRunnerBuf != "/runners/soda-9.0-1" {
		t.Fatalf("umuRunnerBuf = %q after pick, want /runners/soda-9.0-1", m.umuRunnerBuf)
	}
	m.applySettings()
	s, err := settings.Load(root)
	if err != nil {
		t.Fatalf("settings unreadable after apply: %v", err)
	}
	if s.UmuRunnerPath != "/runners/soda-9.0-1" {
		t.Errorf("persisted UmuRunnerPath = %q, want /runners/soda-9.0-1", s.UmuRunnerPath)
	}
}

// TestGUIDetailUmuRunner: the detail panel's per-game runner dropdown
// pins the Proton build for one game immediately (issue 063).
func TestGUIDetailUmuRunner(t *testing.T) {
	root := t.TempDir()
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.SettingsRoot = root
		d.UmuRunners = func() []umu.Runner { return guiTestRunners }
	})
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	headlessFrames(t, 1600, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.runnerDDTriggerRect.Size[0] == 0 {
		t.Fatal("per-game runner trigger not rendered (Linux detail panel must show it)")
	}

	clickRect(m.runnerDDTriggerRect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	items := m.runnerDDItems
	if len(items) != 3 {
		t.Fatalf("runner rows = %d, want 3: %+v", len(items), items)
	}

	clickRect(items[1].rect, m.rootView) // Steam: UMU-Proton-10.0-4
	if got := sess.UmuRunnerPin(row.InstallDir); got != "/runners/UMU-Proton-10.0-4" {
		t.Fatalf("UmuRunnerPin = %q after pick, want /runners/UMU-Proton-10.0-4", got)
	}
	s, err := settings.Load(root)
	if err != nil {
		t.Fatalf("settings unreadable after pin: %v", err)
	}
	if len(s.RunnerOverrides) != 1 {
		t.Fatalf("persisted RunnerOverrides = %v, want exactly one pin", s.RunnerOverrides)
	}
	for dir, r := range s.RunnerOverrides {
		if r != "/runners/UMU-Proton-10.0-4" {
			t.Errorf("RunnerOverrides[%s] = %q, want /runners/UMU-Proton-10.0-4", dir, r)
		}
	}

	// Reopen and pick Auto: the pin clears.
	clickRect(m.runnerDDTriggerRect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	clickRect(m.runnerDDItems[0].rect, m.rootView)
	if got := sess.UmuRunnerPin(row.InstallDir); got != "" {
		t.Errorf("UmuRunnerPin = %q after Auto pick, want empty (cleared)", got)
	}
}
