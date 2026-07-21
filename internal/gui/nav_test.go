package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
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

// seedNavSession scans n fake games into a session and returns it in list
// mode, with the settled visible rows for reference (enrichment re-sorts
// rows while it runs, so the test waits for the session to go quiet).
func seedNavSession(t *testing.T, n int) (*ui.Session, []ui.GameRow) {
	t.Helper()
	sess, _ := guiFakes(t)
	for i := range n {
		dir := filepath.Join(t.TempDir(), fmt.Sprintf("Game%02d", i))
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
	base := 1 // guiFakes pre-seeds the "Game One" Steam row
	deadline := time.Now().Add(15 * time.Second)
	for len(sess.VisibleRows()) < base+n && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	quiet := time.NewTimer(300 * time.Millisecond)
	for {
		select {
		case <-sess.Events():
			if !quiet.Stop() {
				<-quiet.C
			}
			quiet.Reset(300 * time.Millisecond)
		case <-quiet.C:
			rows := sess.VisibleRows()
			if len(rows) != base+n {
				t.Fatalf("rows %d, want %d", len(rows), base+n)
			}
			sess.ToggleView() // grid → list
			return sess, rows
		}
	}
}

// TestListArrowKeyNav: in list mode Up/Down move the selection one row
// (clamped at both ends) and Left/Right do nothing.
func TestListArrowKeyNav(t *testing.T) {
	sess, rows := seedNavSession(t, 5)
	m := newModel(Config{Session: sess})
	last := len(rows) - 1

	headlessFrames(t, 800, 600)
	keyFrame(KeyCodeNone, 0, m.rootView) // build + drain (list mode active)
	if m.selIdx != 0 {
		t.Fatalf("initial selection index %d, want 0", m.selIdx)
	}
	keyFrame(KeyDown, 0, m.rootView)
	if m.selIdx != 1 {
		t.Errorf("after Down: selIdx %d, want 1 (one row, not cols)", m.selIdx)
	}
	keyFrame(KeyDown, 0, m.rootView)
	keyFrame(KeyUp, 0, m.rootView)
	if m.selIdx != 1 {
		t.Errorf("after Down+Up: selIdx %d, want 1", m.selIdx)
	}
	keyFrame(KeyUp, 0, m.rootView)
	keyFrame(KeyUp, 0, m.rootView) // clamp at the top
	if m.selIdx != 0 {
		t.Errorf("after Up x2: selIdx %d, want clamped 0", m.selIdx)
	}
	for range len(rows) + 2 {
		keyFrame(KeyDown, 0, m.rootView)
	}
	if m.selIdx != last {
		t.Errorf("after Down past the end: selIdx %d, want clamped %d", m.selIdx, last)
	}
	keyFrame(KeyRight, 0, m.rootView)
	keyFrame(KeyLeft, 0, m.rootView)
	if m.selIdx != last {
		t.Errorf("Left/Right in list mode: selIdx %d, want unchanged %d", m.selIdx, last)
	}
}

// TestListEnterTogglesDetailPanel: Enter opens the detail panel for the
// selected row; Enter again on the same row closes it; Enter on a
// different row switches the panel to it.
func TestListEnterTogglesDetailPanel(t *testing.T) {
	sess, rows := seedNavSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyEnter, 0, m.rootView)
	if got := sess.Snapshot().Selected; got != rows[0].InstallDir {
		t.Fatalf("after Enter: selected %q, want %q (panel open)", got, rows[0].InstallDir)
	}
	keyFrame(KeyEnter, 0, m.rootView)
	if got := sess.Snapshot().Selected; got != "" {
		t.Errorf("after second Enter on same row: selected %q, want closed (toggle)", got)
	}
	keyFrame(KeyDown, 0, m.rootView)
	keyFrame(KeyEnter, 0, m.rootView)
	if got := sess.Snapshot().Selected; got != rows[1].InstallDir {
		t.Errorf("after Down+Enter: selected %q, want %q", got, rows[1].InstallDir)
	}
}

// TestListSelectionScrollsIntoView: moving the keyboard selection past the
// visible rows scrolls the virtualized list so the selected row renders
// (its rect lands inside the window).
func TestListSelectionScrollsIntoView(t *testing.T) {
	sess, _ := seedNavSession(t, 10)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 700, 300) // short window: only a few rows visible
	keyFrame(KeyCodeNone, 0, m.rootView)
	for range 8 {
		keyFrame(KeyDown, 0, m.rootView)
	}
	if m.selIdx != 8 {
		t.Fatalf("selIdx %d, want 8", m.selIdx)
	}
	RunFrameFn(m.rootView) // consume the scroll-into-view command
	RunFrameFn(m.rootView) // re-render the selected row with real layout data
	r := m.listSelRect
	if r.Size[0] == 0 {
		t.Fatal("selected row never rendered after scrolling past the viewport")
	}
	if r.Origin[1] < 0 || r.Origin[1]+r.Size[1] > 300 {
		t.Errorf("selected row rect %+v outside the 300px window (not scrolled into view)", r)
	}
}

// focusList builds one frame (recording m.listID), then moves keyboard focus
// onto the list wrapper directly and settles a frame so HasFocus is stable.
// It fails the test when the wrapper never takes focus.
func focusList(t *testing.T, m *model) {
	t.Helper()
	keyFrame(KeyCodeNone, 0, m.rootView) // build; captures m.listID
	FocusImmediateOn(m.listID)
	keyFrame(KeyCodeNone, 0, m.rootView) // focus applies at frame start
	if !IdHasFocus(m.listID) {
		t.Fatal("list wrapper did not take focus")
	}
}

// TestListTabFocusesListView: starting from the search field, Tab cycles
// focus in document order through the focusables (search is the last
// toolbar focusable, so the list wrapper is next) and lands on the list.
func TestListTabFocusesListView(t *testing.T) {
	sess, _ := seedNavSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	keyFrame(KeyCodeNone, 0, m.rootView) // build; registers focusables, captures ids
	FocusImmediateOn(m.searchID)
	keyFrame(KeyCodeNone, 0, m.rootView) // focus settles on the search field
	if !IdHasFocus(m.searchID) {
		t.Fatal("search field did not take focus")
	}
	// A focus change requested during a frame applies at the next frame's
	// start, so each Tab is followed by a plain settling frame.
	focused := false
	for range 8 {
		keyFrame(KeyTab, 0, m.rootView)
		keyFrame(KeyCodeNone, 0, m.rootView)
		if IdHasFocus(m.listID) {
			focused = true
			break
		}
	}
	if !focused {
		t.Fatal("Tab never moved focus onto the list view")
	}
}

// TestListFocusedArrowsMoveSelection: with the list focused, each Down/Up
// keyframe moves the selection by exactly one row — the focused handler
// consumes the key, so the global fallback cannot double-fire.
func TestListFocusedArrowsMoveSelection(t *testing.T) {
	sess, _ := seedNavSession(t, 5)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	focusList(t, m)
	if m.selIdx != 0 {
		t.Fatalf("initial selection index %d, want 0", m.selIdx)
	}
	keyFrame(KeyDown, 0, m.rootView)
	if m.selIdx != 1 {
		t.Fatalf("after focused Down: selIdx %d, want exactly 1 (double-fire?)", m.selIdx)
	}
	keyFrame(KeyDown, 0, m.rootView)
	if m.selIdx != 2 {
		t.Fatalf("after focused Down x2: selIdx %d, want exactly 2", m.selIdx)
	}
	keyFrame(KeyUp, 0, m.rootView)
	if m.selIdx != 1 {
		t.Errorf("after focused Up: selIdx %d, want exactly 1", m.selIdx)
	}
}

// TestListFocusedEnterTogglesDetailConsumed: with the list focused, Enter
// toggles the detail panel exactly once per keyframe — open, then closed.
func TestListFocusedEnterTogglesDetailConsumed(t *testing.T) {
	sess, rows := seedNavSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	focusList(t, m)
	keyFrame(KeyEnter, 0, m.rootView)
	if got := sess.Snapshot().Selected; got != rows[0].InstallDir {
		t.Fatalf("after focused Enter: selected %q, want %q (panel open)", got, rows[0].InstallDir)
	}
	keyFrame(KeyEnter, 0, m.rootView)
	if got := sess.Snapshot().Selected; got != "" {
		t.Errorf("after second focused Enter: selected %q, want closed (toggled exactly once)", got)
	}
}

// TestListTabExitsFocus: with the list focused, one Tab moves focus away to
// the next focusable (Tab is never consumed); arrows then move the
// selection through the unfocused global fallback again.
func TestListTabExitsFocus(t *testing.T) {
	sess, _ := seedNavSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	focusList(t, m)
	keyFrame(KeyTab, 0, m.rootView)      // shirei cycles focus to the next sibling
	keyFrame(KeyCodeNone, 0, m.rootView) // focus change applies
	if IdHasFocus(m.listID) {
		t.Fatal("list wrapper still focused after Tab")
	}
	keyFrame(KeyDown, 0, m.rootView) // unfocused: global fallback moves the row
	if m.selIdx != 1 {
		t.Errorf("after Tab out + Down: selIdx %d, want 1 (global fallback)", m.selIdx)
	}
}

// TestListFocusRingVisible: the list wrapper draws its focus ring only
// while it holds keyboard focus (seam: m.listFocusRing is captured when the
// wrapper applies the focus border color during the build).
func TestListFocusRingVisible(t *testing.T) {
	sess, _ := seedNavSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	keyFrame(KeyCodeNone, 0, m.rootView) // build, nothing focused
	if m.listFocusRing {
		t.Error("focus ring drawn while the list is not focused")
	}
	FocusImmediateOn(m.listID)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if !IdHasFocus(m.listID) {
		t.Fatal("list wrapper did not take focus")
	}
	if !m.listFocusRing {
		t.Error("focus ring not drawn while the list is focused")
	}
}
