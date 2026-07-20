package gui

import (
	"context"
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// clickRect runs a full click gesture (hover settle, down, release) at the
// center of rect across three frames.
func clickRect(rect Rect, fn FrameFn) {
	InputState.MousePoint = Vec2{rect.Origin[0] + rect.Size[0]/2, rect.Origin[1] + rect.Size[1]/2}
	RunFrameFn(fn) // hover settles from the previous frame's hoverables
	FrameInput.Mouse = MouseClick
	RunFrameFn(fn)
	FrameInput.Mouse = MouseRelease
	RunFrameFn(fn)
	FrameInput.Mouse = 0
}

// TestGridTrailingSpacer: the grid appends a trailing spacer row so the last
// card row never renders flush against the viewport edge.
func TestGridTrailingSpacer(t *testing.T) {
	if got := gridItemCount(3); got != 4 {
		t.Errorf("gridItemCount(3) = %d, want 4 (3 chunks + spacer)", got)
	}
	if got := gridItemCount(0); got != 1 {
		t.Errorf("gridItemCount(0) = %d, want 1 (spacer only)", got)
	}
}

// TestGUIToolbarControlsDisabledWhenEmpty: with an empty library the view
// switch ignores clicks; with rows it toggles the view mode.
func TestGUIToolbarControlsDisabledWhenEmpty(t *testing.T) {
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})
	m.state = ui.State{Mode: ui.ViewGrid, StatusLine: "0 games"} // empty library

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture segment rect
	r := m.listSegRect
	if r.Size[0] == 0 {
		t.Fatalf("list segment rect not recorded: %+v", r)
	}
	clickRect(r, m.rootView)
	if got := sess.Snapshot().Mode; got != ui.ViewGrid {
		t.Errorf("empty library: view switch click changed mode to %v, want disabled", got)
	}
	t.Log("empty library: view switch inert")

	// With a scanned row the same click toggles to list mode.
	sess2, _ := guiFakes(t)
	sess2.Scan(context.Background())
	deadline := time.Now().Add(15 * time.Second)
	for len(sess2.VisibleRows()) == 0 && time.Now().Before(deadline) {
		select {
		case <-sess2.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	m2 := newModel(Config{Session: sess2})
	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m2.rootView)
	keyFrame(KeyCodeNone, 0, m2.rootView)
	clickRect(m2.listSegRect, m2.rootView)
	if got := sess2.Snapshot().Mode; got != ui.ViewList {
		t.Errorf("populated library: view switch click left mode %v, want ViewList", got)
	}
	t.Log("populated library: view switch toggles")
}
