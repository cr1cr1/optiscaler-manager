package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestDetailPanelWidth_Clamps: the detail panel tracks 30% of the window
// width, clamped to [300, 480] so narrow windows keep a usable panel and
// ultrawide windows do not grow an absurd sidebar.
func TestDetailPanelWidth_Clamps(t *testing.T) {
	tests := []struct {
		windowW float32
		want    float32
	}{
		{800, 300},  // 240 clamps up to the floor
		{1100, 330}, // proportional inside the band
		{2000, 480}, // 600 clamps down to the ceiling
	}
	for _, tt := range tests {
		if got := detailPanelWidth(tt.windowW); got != tt.want {
			t.Errorf("detailPanelWidth(%v) = %v, want %v", tt.windowW, got, tt.want)
		}
	}
}

// TestDetailPanelResolvesToProportionalWidth: the rendered panel shell is
// exactly detailPanelWidth wide — the scrollable column must not absorb the
// Row's leftover space (Viewport on a Row child defeats FixWidth).
func TestDetailPanelResolvesToProportionalWidth(t *testing.T) {
	m := newModel(Config{})
	m.state = ui.State{
		Mode:       ui.ViewGrid,
		StatusLine: "1 games",
		Rows:       []ui.GameRow{{Title: "Panel Game", InstallDir: "/games/panel", AppID: "1"}},
		Selected:   "/games/panel",
	}

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame

	r := m.detailPanelRect
	if r.Size[0] == 0 {
		t.Fatalf("detail panel rect not recorded: %+v", r)
	}
	if want := detailPanelWidth(1100); r.Size[0] != want {
		t.Errorf("detail panel width %v, want %v (rect %+v)", r.Size[0], want, r)
	}
	t.Logf("detail panel resolves to %v at 1100px window", r.Size[0])
}
