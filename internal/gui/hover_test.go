package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestGUICardHoverState: hovering a card applies the hover treatment and
// records the hovered game on the model; moving the mouse away clears it.
func TestGUICardHoverState(t *testing.T) {
	m := newModel(Config{})
	row := ui.GameRow{Title: "Hover Game", InstallDir: "/games/hover", AppID: "7"}

	headlessFrames(t, 400, 700)
	InputState.MousePoint = Vec2{-50, -50}
	view := func() {
		Container(Attrs(Viewport), func() {
			m.fitCards(400)
			m.gameCard(row)
		})
	}
	keyFrame(KeyCodeNone, 0, view) // mouse parked outside
	if m.hoveredDir != "" {
		t.Fatalf("hoveredDir %q with mouse outside, want empty", m.hoveredDir)
	}

	r := m.cardRect
	if r.Size[0] == 0 || r.Size[1] == 0 {
		t.Fatalf("card rect not recorded: %+v", r)
	}
	InputState.MousePoint = Vec2{r.Origin[0] + r.Size[0]/2, r.Origin[1] + r.Size[1]/2}
	keyFrame(KeyCodeNone, 0, view)
	if m.hoveredDir != row.InstallDir {
		t.Errorf("hoveredDir %q with mouse over card, want %q", m.hoveredDir, row.InstallDir)
	}

	InputState.MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, view)
	if m.hoveredDir != "" {
		t.Errorf("hoveredDir %q after mouse left, want cleared", m.hoveredDir)
	}
	t.Logf("card hover tracked over rect %+v", r)
}
