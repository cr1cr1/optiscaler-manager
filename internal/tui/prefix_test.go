package tui

import (
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
	if !strings.Contains(view, "SomeApp (heroic)") {
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
	if !strings.Contains(view, "SomeApp (heroic)") {
		t.Errorf("reopened picker view lacks prefix row:\n%s", view)
	}
}
