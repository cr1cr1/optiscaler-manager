package gui

import (
	"testing"

	. "go.hasen.dev/shirei"
)

// TestDetailOpenFolderButton (issue 027): the detail panel always renders
// the Open-game-folder button — install state is irrelevant (the folder
// exists either way), unlike OpenINI which is gated on CanOpenINI. The
// rect seam mirrors openINIRect: non-zero when rendered.
func TestDetailOpenFolderButton(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	if row.CanOpenINI() {
		t.Fatalf("row CanOpenINI() = true (status %q); a clean row proves the button is not install-gated", row.Status)
	}
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	// Tall window: cover art pushes the action buttons near the fold, and
	// shirei culls clipped Viewport children (zero rect).
	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame
	if m.openFolderRect.Size[0] == 0 || m.openFolderRect.Size[1] == 0 {
		t.Errorf("Open game folder button not rendered for a clean row (rect %+v)", m.openFolderRect)
	}
	t.Logf("open-folder button rect: %+v", m.openFolderRect)
}
