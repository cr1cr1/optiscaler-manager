package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
	"github.com/cr1cr1/optiscaler-manager/internal/umu"
)

// TestGUIDetailPrefixDropdown: the detail panel's per-game prefix
// dropdown lists Auto (inherit) + the detected prefixes; picking one
// pins it via SetUmuPrefixOverride (persisted per install dir), and
// picking Auto clears the pin again.
func TestGUIDetailPrefixDropdown(t *testing.T) {
	prefixes := []umu.Prefix{
		{Name: "SomeApp", Path: "/home/me/Games/Heroic/Prefixes/SomeApp", Source: "heroic"},
		{Name: "mybottle", Path: "/home/me/.local/share/bottles/bottles/mybottle", Source: "bottles"},
	}
	root := t.TempDir()
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.SettingsRoot = root
		d.UmuPrefixes = func() []umu.Prefix { return prefixes }
	})
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	headlessFrames(t, 1600, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.prefixDDTriggerRect.Size[0] == 0 {
		t.Fatal("prefix dropdown trigger not rendered (Linux detail panel must show it)")
	}

	clickRect(m.prefixDDTriggerRect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	items := m.prefixDDItems
	if len(items) != 3 {
		t.Fatalf("dropdown rows = %d, want 3 (Auto + 2 prefixes): %+v", len(items), items)
	}
	wantLabels := []string{"Auto (inherit)", "SomeApp (heroic)", "mybottle (bottles)"}
	for i, w := range wantLabels {
		if items[i].label != w {
			t.Errorf("items[%d].label = %q, want %q", i, items[i].label, w)
		}
	}

	clickRect(items[1].rect, m.rootView) // pin the heroic prefix
	if got := sess.UmuPrefixPin(row.InstallDir); got != prefixes[0].Path {
		t.Fatalf("UmuPrefixPin = %q after pick, want %q", got, prefixes[0].Path)
	}
	s, err := settings.Load(root)
	if err != nil {
		t.Fatalf("settings unreadable after pin: %v", err)
	}
	if len(s.PrefixOverrides) != 1 {
		t.Fatalf("persisted PrefixOverrides = %v, want exactly one pin", s.PrefixOverrides)
	}
	for dir, p := range s.PrefixOverrides {
		if p != prefixes[0].Path {
			t.Errorf("PrefixOverrides[%s] = %q, want %q", dir, p, prefixes[0].Path)
		}
	}

	// Reopen and pick Auto: the pin clears.
	clickRect(m.prefixDDTriggerRect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	clickRect(m.prefixDDItems[0].rect, m.rootView)
	if got := sess.UmuPrefixPin(row.InstallDir); got != "" {
		t.Errorf("UmuPrefixPin = %q after Auto pick, want empty (cleared)", got)
	}
	t.Logf("per-game prefix pin set and cleared for %s", row.InstallDir)
}
