package gui

import (
	"strings"
	"testing"
	"time"

	. "go.hasen.dev/shirei"
)

// The detail pane hosts a per-game Rescan button (issue 035): same refresh
// pipeline as the toolbar Scan, scoped to the selected game. Rendered for
// any row — clean or installed — and its activation settles a rescan
// toast from the session.
func TestDetailRescanButton(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	// Tall window: cover art pushes the action buttons near the fold, and
	// shirei culls clipped Viewport children (zero rect).
	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame
	if m.rescanBtnRect.Size[0] == 0 || m.rescanBtnRect.Size[1] == 0 {
		t.Fatalf("Rescan button not rendered for a clean row (rect %+v)", m.rescanBtnRect)
	}

	clickRect(m.rescanBtnRect, m.rootView)

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, to := range sess.Snapshot().Toasts {
			if strings.HasPrefix(to.Text, "rescanned ") {
				t.Logf("rescan settled via the button: %q", to.Text)
				return
			}
		}
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Error("clicking Rescan did not run a per-game rescan (no settle toast)")
}
