package gui

import (
	"testing"

	. "go.hasen.dev/shirei"
)

// TestSidebarItems_UniformWidth: every nav item resolves to the same width
// (sidebar inner width), so the icon column does not auto-size raggedly per
// label length.
func TestSidebarItems_UniformWidth(t *testing.T) {
	m := newModel(Config{})

	headlessFrames(t, 800, 600)
	view := func() {
		Container(Attrs(Viewport, Row), func() {
			m.sidebar()
		})
	}
	keyFrame(KeyCodeNone, 0, view) // build
	keyFrame(KeyCodeNone, 0, view) // capture rects from the previous frame

	if len(m.sidebarRects) != 4 {
		t.Fatalf("sidebar item rects %d, want 4 (Games, Prefs, About, Exit)", len(m.sidebarRects))
	}
	want := float32(sidebarW - 2*sp8)
	for i, r := range m.sidebarRects {
		if r.Size[0] != want {
			t.Errorf("sidebar item %d width %v, want uniform %v (rects: %+v)", i, r.Size[0], want, m.sidebarRects)
		}
	}
	t.Logf("sidebar item widths uniform at %v", want)
}
