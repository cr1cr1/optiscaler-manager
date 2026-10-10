package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
	"github.com/cr1cr1/optiscaler-manager/internal/umu"
)

// tuiTestPrefixes is the shared detected-prefix fixture for the TUI
// picker tests.
var tuiTestPrefixes = []umu.Prefix{
	{Name: "SomeApp", Path: "/home/me/Games/Heroic/Prefixes/SomeApp", Source: "heroic"},
	{Name: "mybottle", Path: "/home/me/.local/share/bottles/bottles/mybottle", Source: "bottles"},
}

// TestTUIPrefixPickerStateMachine: drive the model directly (no
// teatest) through open → pick → reopen to isolate the picker state
// machine from terminal rendering.
func TestTUIPrefixPickerStateMachine(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) {
		d.UmuPrefixes = func() []umu.Prefix { return tuiTestPrefixes }
	})
	var tm tea.Model = New(e.sess, "test")
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	key := func(s string) {
		t.Helper()
		m, _ := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
		tm = m
	}
	key("2") // settings screen
	key("p") // open picker
	m := tm.(Model)
	if m.prefix == nil {
		t.Fatal("picker did not open on p")
	}
	if m.prefix.sel != 0 {
		t.Errorf("initial sel = %d, want 0 (Auto)", m.prefix.sel)
	}
	view := tm.(Model).View()
	if !strings.Contains(view, "Heroic: SomeApp") {
		t.Errorf("picker view lacks prefix row:\n%s", view)
	}

	key("j")
	enter, _ := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = enter
	if got := e.sess.Settings().UmuPrefixPath; got != "/home/me/Games/Heroic/Prefixes/SomeApp" {
		t.Fatalf("UmuPrefixPath = %q after pick, want the heroic prefix", got)
	}

	key("p") // reopen
	m = tm.(Model)
	if m.prefix == nil {
		t.Fatal("picker did not reopen on p")
	}
	if m.prefix.sel != 1 {
		t.Errorf("reopen sel = %d, want 1 (current pick)", m.prefix.sel)
	}
	view = tm.(Model).View()
	if !strings.Contains(view, "Heroic: SomeApp") {
		t.Errorf("reopened picker view lacks prefix row:\n%s", view)
	}
}

// TestPrefixBoxWindowsAtTwenty: a long prefix list renders at most 20
// rows, the window following the selection so the rest scroll (issue 056).
func TestPrefixBoxWindowsAtTwenty(t *testing.T) {
	items := make([]ui.UmuPickerOption, 25)
	for i := range items {
		items[i] = ui.UmuPickerOption{Value: fmt.Sprint(i), Source: "bottles", Name: fmt.Sprintf("pfx-%02d", i)}
	}

	top := prefixBox(&prefixPick{items: items, sel: 0})
	if !strings.Contains(top, "pfx-00") || !strings.Contains(top, "pfx-19") {
		t.Errorf("top window must show rows 0..19:\n%s", top)
	}
	if strings.Contains(top, "pfx-20") {
		t.Errorf("top window shows a 21st row — the cap is 20:\n%s", top)
	}

	bottom := prefixBox(&prefixPick{items: items, sel: 24})
	if !strings.Contains(bottom, "pfx-24") {
		t.Errorf("bottom window must follow the selection to row 24:\n%s", bottom)
	}
	if strings.Contains(bottom, "pfx-04") {
		t.Errorf("bottom window still shows row 4 — the window must scroll:\n%s", bottom)
	}
}

// TestTUIRouteCycleKeys: r on settings cycles the global launch route
// (auto → umu → owner → auto) and e on the detail screen cycles the
// per-game override without touching the global default (issue 060).
func TestTUIRouteCycleKeys(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) {
		d.SettingsRoot = t.TempDir()
	})
	var tm tea.Model = New(e.sess, "test")
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	key := func(s string) {
		t.Helper()
		m, _ := tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
		tm = m
	}

	e.sess.Scan(t.Context())
	pollUntil(t, "scan finds GameOne", func() bool {
		return len(e.sess.Snapshot().Rows) > 0
	})

	key("2") // settings screen
	for i, want := range []string{"umu", "owner", ""} {
		key("r")
		if got := e.sess.Settings().LaunchRoute; got != want {
			t.Errorf("after %d r presses LaunchRoute = %q, want %q", i+1, got, want)
		}
	}
	view := tm.(Model).View()
	if !strings.Contains(view, "umu launch route:") {
		t.Errorf("settings view lacks the route row:\n%s", view)
	}

	// Detail screen: e cycles the per-game override.
	key("1") // games
	enter, _ := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = enter // detail screen
	key("e")
	if got := e.sess.LaunchRoutePin(e.gameRoot); got != "umu" {
		t.Errorf("LaunchRoutePin = %q after e, want umu", got)
	}
	if got := e.sess.Settings().LaunchRoute; got != "" {
		t.Errorf("global LaunchRoute = %q after per-game cycle, want untouched", got)
	}
	key("e")
	key("e")
	if got := e.sess.LaunchRoutePin(e.gameRoot); got != "" {
		t.Errorf("LaunchRoutePin = %q after 3 e presses, want cleared (auto)", got)
	}
}
