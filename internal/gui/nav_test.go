package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "go.hasen.dev/shirei"
)

// TestGUIArrowKeyNav: arrow keys move the grid selection (±1 horizontal,
// ±cols vertical, clamped), Enter opens the detail view, Escape closes it.
func TestGUIArrowKeyNav(t *testing.T) {
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})

	for _, name := range []string{"Bravo", "Charlie"} {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// A real exe makes the dir a game (v0.7): empty dirs are refused.
		if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MZGAME"), 0o644); err != nil {
			t.Fatal(err)
		}
		sess.AddDirectory(dir)
	}
	sess.Scan(context.Background())
	deadline := time.Now().Add(15 * time.Second)
	for len(sess.VisibleRows()) < 3 && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	rows := sess.VisibleRows()
	if len(rows) != 3 {
		t.Fatalf("rows %d, want 3", len(rows))
	}

	headlessFrames(t, 800, 600)
	keyFrame(KeyCodeNone, 0, m.rootView) // build; derives m.cols from live width
	m.drain()
	if m.selIdx != 0 {
		t.Fatalf("initial selection index %d, want 0", m.selIdx)
	}

	keyFrame(KeyRight, 0, m.rootView)
	if m.selIdx != 1 {
		t.Errorf("after Right: selIdx %d, want 1", m.selIdx)
	}
	keyFrame(KeyRight, 0, m.rootView)
	keyFrame(KeyRight, 0, m.rootView) // clamp at the last card
	if m.selIdx != 2 {
		t.Errorf("after Right x3: selIdx %d, want clamped 2", m.selIdx)
	}
	keyFrame(KeyDown, 0, m.rootView) // +cols also clamps
	if m.selIdx != 2 {
		t.Errorf("after Down: selIdx %d, want clamped 2", m.selIdx)
	}
	keyFrame(KeyUp, 0, m.rootView) // -cols lands back at the top row
	if want := 2 - m.cols; m.selIdx != max(want, 0) {
		t.Errorf("after Up: selIdx %d, want %d (cols=%d)", m.selIdx, max(want, 0), m.cols)
	}
	keyFrame(KeyLeft, 0, m.rootView)
	keyFrame(KeyLeft, 0, m.rootView) // clamp at 0
	if m.selIdx != 0 {
		t.Errorf("after Left x2: selIdx %d, want clamped 0", m.selIdx)
	}

	keyFrame(KeyEnter, 0, m.rootView) // open detail for the selected card
	if got := sess.Snapshot().Selected; got != rows[0].InstallDir {
		t.Errorf("after Enter: selected %q, want %q", got, rows[0].InstallDir)
	}
	m.drain()
	keyFrame(KeyEscape, 0, m.rootView) // close the detail view
	if got := sess.Snapshot().Selected; got != "" {
		t.Errorf("after Escape: selected %q, want closed", got)
	}
	t.Logf("arrow nav: cols=%d, open/close via Enter/Escape", m.cols)
}
