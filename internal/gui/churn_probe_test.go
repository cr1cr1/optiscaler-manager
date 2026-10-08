package gui

import (
	"testing"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"
)

// Probe: after a deep-card click opens the panel (issue 037 scenario),
// how many frames stay hot? Healthy: quiet after ~5. Churn: repeated
// NextFrameRequested/FrameHasChanges with scrollCursorPending re-arms.
func TestScrollChurnProbe(t *testing.T) {
	sess, rows := fourteenGameLibrary(t)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1100, 800)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	cols := m.cols

	VirtualListScrollIntoView("grid", (len(rows)-1)/cols)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	last := rows[len(rows)-1]
	if m.cardIDs[last.InstallDir] == nil {
		t.Fatalf("last-row card not visible; setup broken")
	}
	card := m.cardRect
	clickRect(card, m.rootView)
	if sess.Snapshot().Selected != last.InstallDir {
		t.Fatal("click did not select")
	}

	hot := 0
	for i := 0; i < 40; i++ {
		GetFrameInput().Key = KeyCodeNone
		out := RunFrameFn(m.rootView)
		if out.FrameHasChanges || out.NextFrameRequested {
			hot++
			t.Logf("frame %d: changes=%v req=%v scrollPending=%v focusPending=%q cols=%d selIdx=%d",
				i, out.FrameHasChanges, out.NextFrameRequested, m.scrollCursorPending, m.cardFocusPending, m.cols, m.selIdx)
		}
	}
	t.Logf("hot frames: %d/40", hot)
	if hot > 10 {
		t.Errorf("churn: %d hot frames after panel-open settle", hot)
	}
}
