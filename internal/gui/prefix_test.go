package gui

import (
	"fmt"
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
	wantLabels := []string{"Auto (inherit)", "Heroic: SomeApp", "Bottles: mybottle"}
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

// TestGUIDetailPrefixDropdownScrolls: with more than 20 rows the popup
// renders a 20-row window and the rest scroll — by wheel and by the
// keyboard highlight pulling the window along (issue 056).
func TestGUIDetailPrefixDropdownScrolls(t *testing.T) {
	prefixes := make([]umu.Prefix, 25)
	for i := range prefixes {
		prefixes[i] = umu.Prefix{Name: fmt.Sprintf("pfx-%02d", i), Path: fmt.Sprintf("/pfx/%02d", i), Source: "bottles"}
	}
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.SettingsRoot = t.TempDir()
		d.UmuPrefixes = func() []umu.Prefix { return prefixes }
	})
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	headlessFrames(t, 1600, 1400)
	GetInputState().MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	clickRect(m.prefixDDTriggerRect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)

	items := m.prefixDDItems // Auto + 25 = 26 rows, windowed at 20
	if len(items) != 20 {
		t.Fatalf("rendered rows = %d, want the 20-row cap", len(items))
	}
	if items[0].label != "Auto (inherit)" || items[19].label != "Bottles: pfx-18" {
		t.Errorf("initial window = %q .. %q, want Auto .. Bottles: pfx-18", items[0].label, items[19].label)
	}
	menu := m.prefixDDMenuRect
	pitch := items[1].rect.Origin[1] - items[0].rect.Origin[1]
	if pitch <= 0 {
		t.Fatalf("row pitch = %v, want > 0", pitch)
	}
	if got, want := menu.Size[1], 20*pitch+2*sp4; got > want+1 || got < want-2 {
		t.Errorf("popup height = %v, want 20 rows (%v)", got, want)
	}

	// Keyboard to the last row: the window follows the highlight. hl
	// starts on row 0, so 25 Downs land on row 25 (26 would wrap to 0).
	for i := 0; i < 25; i++ {
		keyFrame(KeyDown, 0, m.rootView)
	}
	items = m.prefixDDItems
	if len(items) != 20 || items[19].label != "Bottles: pfx-24" || !items[19].hl {
		t.Fatalf("window did not follow the highlight to the bottom: %+v", items)
	}
	if items[0].label == "Auto (inherit)" {
		t.Error("window still starts at the top after keyboarding to the bottom")
	}

	// Wheel over the popup scrolls the window back up (and only the
	// popup: the scroll is consumed).
	GetInputState().MousePoint = Vec2{menu.Origin[0] + menu.Size[0]/2, menu.Origin[1] + menu.Size[1]/2}
	keyFrame(KeyCodeNone, 0, m.rootView) // hover settles
	GetFrameInput().Scroll = Vec2{0, -600}
	RunFrameFn(m.rootView)
	GetFrameInput().Scroll = Vec2{}
	keyFrame(KeyCodeNone, 0, m.rootView)
	items = m.prefixDDItems
	if items[0].label != "Auto (inherit)" {
		t.Errorf("wheel did not scroll back to the top: window starts at %q", items[0].label)
	}
	// Clicking works on scrolled rows too: pick the last visible prefix.
	GetInputState().MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView)
	clickRect(m.prefixDDItems[19].rect, m.rootView)
	if got := sess.UmuPrefixPin(row.InstallDir); got != "/pfx/18" {
		t.Errorf("UmuPrefixPin = %q after picking a windowed row, want /pfx/18", got)
	}
}
