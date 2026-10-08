package gui

import (
	"strings"
	"testing"
	"time"

	. "go.hasen.dev/shirei"
)

// Issue 052: the status bar stays one line tall regardless of the status
// text (multi-line umu stderr tails used to grow it vertically), and a
// Log button on its left slides up a readonly, selectable log panel.

func TestStatusBarHeightIgnoresMultilineStatus(t *testing.T) {
	headlessFrames(t, 1100, 700)
	m := newModel(Config{})
	m.state.StatusLine = "Ready"
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects
	h1 := m.statusBarRect.Size[1]
	if h1 == 0 {
		t.Fatal("status bar rect not captured")
	}

	m.state.StatusLine = "Launch failed: boom\nstderr two\nstderr three\nstderr four"
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	h2 := m.statusBarRect.Size[1]
	if h2 != h1 {
		t.Errorf("status bar grew with a multi-line status: %v -> %v", h1, h2)
	}
}

func TestLogButtonTogglesPanel(t *testing.T) {
	headlessFrames(t, 1100, 700)
	m := newModel(Config{})
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.logBtnRect.Size[0] == 0 {
		t.Fatal("Log button not rendered in the status bar")
	}
	if m.logPanelRect.Size[1] != 0 {
		t.Fatalf("log panel visible before toggle: %+v", m.logPanelRect)
	}

	m.toggleLogPanel() // open; jump the tween to its end
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.logPanelRect.Size[1] < 100 {
		t.Fatalf("log panel not open after toggle: %+v", m.logPanelRect)
	}

	m.toggleLogPanel() // close; jump the tween again
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.logPanelRect.Size[1] != 0 {
		t.Fatalf("log panel still visible after second toggle: %+v", m.logPanelRect)
	}
}

func TestEscClosesLogPanelFirst(t *testing.T) {
	headlessFrames(t, 1100, 700)
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})
	m.toggleLogPanel()
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView) // build (drains session state)
	keyFrame(KeyEscape, 0, m.rootView)   // Esc closes the log panel first
	if m.logOpen {
		t.Error("Esc did not close the log panel")
	}
}

func TestLogPanelShowsSessionLines(t *testing.T) {
	headlessFrames(t, 1100, 700)
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})

	sess.Launch("/definitely/not/a/game")
	deadline := time.Now().Add(2 * time.Second)
	for len(sess.LogLines()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(sess.LogLines()) == 0 {
		t.Fatal("session recorded no log line for the failed launch")
	}

	m.toggleLogPanel()
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView) // syncs the ring
	keyFrame(KeyCodeNone, 0, m.rootView)
	found := false
	for i := 0; i < m.logRing.Len(); i++ {
		if strings.Contains(m.logRing.Line(i), "unknown game") {
			found = true
		}
	}
	if !found {
		t.Fatalf("log ring missing the launch failure; %d lines synced", m.logRing.Len())
	}
}

func TestSyncLogLinesAppendsAndRebuilds(t *testing.T) {
	m := newModel(Config{})
	m.syncLogLines([]string{"a", "b"})
	m.syncLogLines([]string{"a", "b", "c"})
	if m.logRing.Len() != 3 {
		t.Fatalf("after appends: %d lines, want 3", m.logRing.Len())
	}
	// Shorter source (session ring evicted) forces a rebuild.
	m.syncLogLines([]string{"x", "y"})
	if m.logRing.Len() != 2 || m.logRing.Line(0) != "x" || m.logRing.Line(1) != "y" {
		t.Fatalf("rebuild wrong: %d lines, first %q", m.logRing.Len(), m.logRing.Line(0))
	}
}
