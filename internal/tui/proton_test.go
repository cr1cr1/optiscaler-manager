package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
	"github.com/cr1cr1/optiscaler-manager/internal/umu"
)

// TestTUIProtonPickerStateMachine: drive the model directly (no
// teatest) through open → pick → reopen to isolate the picker state
// machine from terminal rendering.
func TestTUIProtonPickerStateMachine(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) {
		d.UmuRunners = func() []umu.Runner {
			return []umu.Runner{
				{Name: "GE-Proton10-1", Path: "/runners/GE-Proton10-1", Source: "heroic", Version: "10.1"},
			}
		}
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
	if m.proton == nil {
		t.Fatal("picker did not open on p")
	}
	if m.proton.sel != 0 {
		t.Errorf("initial sel = %d, want 0 (Auto)", m.proton.sel)
	}
	view := tm.(Model).View()
	if !strings.Contains(view, "GE-Proton10-1 (heroic)") {
		t.Errorf("picker view lacks runner row:\n%s", view)
	}

	key("j")
	enter, _ := tm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm = enter
	if got := e.sess.Settings().UmuProtonPath; got != "/runners/GE-Proton10-1" {
		t.Fatalf("UmuProtonPath = %q after pick, want /runners/GE-Proton10-1", got)
	}

	key("p") // reopen
	m = tm.(Model)
	if m.proton == nil {
		t.Fatal("picker did not reopen on p")
	}
	if m.proton.sel != 1 {
		t.Errorf("reopen sel = %d, want 1 (current pick)", m.proton.sel)
	}
	view = tm.(Model).View()
	if !strings.Contains(view, "GE-Proton10-1 (heroic)") {
		t.Errorf("reopened picker view lacks runner row:\n%s", view)
	}
}
