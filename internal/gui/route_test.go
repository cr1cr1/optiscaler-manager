package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestGUISettingsLaunchRoute: the global launch route is a 3-row
// dropdown (Auto / umu-run / owner CLI); a pick buffers and
// applySettings persists (issue 060).
func TestGUISettingsLaunchRoute(t *testing.T) {
	root := t.TempDir()
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.SettingsRoot = root
		d.Settings = settings.Defaults()
	})
	m := newModel(Config{Session: sess})
	m.openSettings()

	headlessFrames(t, 1000, 700)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.umuRouteTriggerRect.Size[0] == 0 {
		t.Fatal("launch-route trigger not rendered in settings")
	}

	clickRect(m.umuRouteTriggerRect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	items := m.umuRouteMenuItems
	if len(items) != 3 {
		t.Fatalf("route rows = %d, want 3: %+v", len(items), items)
	}
	wantLabels := []string{"Auto (route to the prefix owner)", "umu-run only", "Prefix owner's CLI only"}
	for i, w := range wantLabels {
		if items[i].label != w {
			t.Errorf("items[%d].label = %q, want %q", i, items[i].label, w)
		}
	}

	clickRect(items[1].rect, m.rootView) // umu-run only
	if m.umuRouteBuf != "umu" {
		t.Fatalf("umuRouteBuf = %q after pick, want umu", m.umuRouteBuf)
	}
	m.applySettings()
	s, err := settings.Load(root)
	if err != nil {
		t.Fatalf("settings unreadable after apply: %v", err)
	}
	if s.LaunchRoute != "umu" {
		t.Errorf("persisted LaunchRoute = %q, want umu", s.LaunchRoute)
	}
}

// TestGUIDetailLaunchRoute: the detail panel's per-game launch-route
// dropdown pins the route for one game immediately (issue 060).
func TestGUIDetailLaunchRoute(t *testing.T) {
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
	if m.routeDDTriggerRect.Size[0] == 0 {
		t.Fatal("per-game launch-route trigger not rendered (Linux detail panel must show it)")
	}

	clickRect(m.routeDDTriggerRect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	items := m.routeDDItems
	if len(items) != 3 {
		t.Fatalf("route rows = %d, want 3: %+v", len(items), items)
	}

	clickRect(items[2].rect, m.rootView) // Prefix owner's CLI only
	if got := sess.LaunchRoutePin(row.InstallDir); got != "owner" {
		t.Fatalf("LaunchRoutePin = %q after pick, want owner", got)
	}
	s, err := settings.Load(root)
	if err != nil {
		t.Fatalf("settings unreadable after pin: %v", err)
	}
	if len(s.LaunchRouteOverrides) != 1 {
		t.Fatalf("persisted LaunchRouteOverrides = %v, want exactly one pin", s.LaunchRouteOverrides)
	}
	for dir, r := range s.LaunchRouteOverrides {
		if r != "owner" {
			t.Errorf("LaunchRouteOverrides[%s] = %q, want owner", dir, r)
		}
	}

	// Reopen and pick Auto: the pin clears.
	clickRect(m.routeDDTriggerRect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	clickRect(m.routeDDItems[0].rect, m.rootView)
	if got := sess.LaunchRoutePin(row.InstallDir); got != "" {
		t.Errorf("LaunchRoutePin = %q after Auto pick, want empty (cleared)", got)
	}
}
