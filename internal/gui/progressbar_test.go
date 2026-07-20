package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestProgressBar_RendersDuringScan: while State.Progress is set the scan
// progress bar renders with the fill tracking Done/Total; with a nil
// Progress the bar is hidden.
func TestProgressBar_RendersDuringScan(t *testing.T) {
	m := newModel(Config{})
	m.state = ui.State{
		Mode:       ui.ViewGrid,
		StatusLine: "Scanning…",
		Progress:   &ui.ScanProgress{Phase: "covers", Done: 3, Total: 10},
	}

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame

	track, fill := m.progressTrackRect, m.progressFillRect
	if track.Size[0] == 0 || track.Size[1] == 0 {
		t.Fatalf("progress bar track not rendered while Progress is set: %+v", track)
	}
	ratio := fill.Size[0] / track.Size[0]
	if ratio < 0.25 || ratio > 0.35 {
		t.Errorf("fill ratio %v, want ~0.3 for 3/10 (track %+v fill %+v)", ratio, track, fill)
	}

	m.state.Progress = nil
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.progressTrackRect.Size[0] != 0 {
		t.Errorf("progress bar still visible with nil Progress: %+v", m.progressTrackRect)
	}
	t.Logf("progress bar fill ratio %.2f for 3/10; hidden when idle", ratio)
}
