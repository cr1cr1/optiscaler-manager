package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// seedGridSession seeds n games like seedNavSession but leaves the session
// in grid mode (seedNavSession toggles grid → list; this toggles back).
func seedGridSession(t *testing.T, n int) (*ui.Session, []ui.GameRow) {
	t.Helper()
	sess, rows := seedNavSession(t, n)
	sess.ToggleView() // list → grid
	return sess, rows
}

// focusGrid builds one frame (recording m.gridID), then moves keyboard focus
// onto the grid wrapper directly and settles a frame so HasFocus is stable.
// It fails the test when the wrapper never takes focus. Mirrors focusList.
func focusGrid(t *testing.T, m *model) {
	t.Helper()
	keyFrame(KeyCodeNone, 0, m.rootView) // build; captures m.gridID
	FocusImmediateOn(m.gridID)
	keyFrame(KeyCodeNone, 0, m.rootView) // focus applies at frame start
	if !IdHasFocus(m.gridID) {
		t.Fatal("grid wrapper did not take focus")
	}
}

// TestGrid_TabFocusesGridView: starting from the search field, Tab cycles
// focus in document order through the focusables (search is the last
// toolbar focusable, so the grid wrapper is next) and lands on the grid,
// which draws its focus ring while focused. Mirrors TestListTabFocusesListView.
func TestGrid_TabFocusesGridView(t *testing.T) {
	sess, _ := seedGridSession(t, 3)
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
		if IdHasFocus(m.gridID) {
			focused = true
			break
		}
	}
	if !focused {
		t.Fatal("Tab never moved focus onto the grid view")
	}
	if !m.gridFocusRing {
		t.Error("focus ring not drawn while the grid is focused")
	}
}

// TestGrid_CardClickFocusesSelectsAndMovesCursor: clicking a card body
// selects the game (detail panel), moves the keyboard cursor onto the
// clicked card, and puts keyboard focus on the grid wrapper — the card
// gesture must not leave the grid unfocused with the cursor stranded on
// another card. Mirrors TestListRowClickFocusesAndSelects.
func TestGrid_CardClickFocusesSelectsAndMovesCursor(t *testing.T) {
	sess, rows := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	// Wide window: all three cards land in one chunk, so the last rendered
	// card (m.cardRect is overwritten per card) is the last row.
	headlessFrames(t, 1200, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build; captures m.gridID
	keyFrame(KeyCodeNone, 0, m.rootView) // capture card rects from the previous frame
	k := len(rows) - 1
	if m.cardRect.Size[0] == 0 {
		t.Fatalf("card rect not recorded: %+v", m.cardRect)
	}
	clickRect(m.cardRect, m.rootView)

	if got := sess.Snapshot().Selected; got != rows[k].InstallDir {
		t.Errorf("card click Selected %q, want %q", got, rows[k].InstallDir)
	}
	if m.selIdx != k {
		t.Errorf("card click left cursor selIdx %d, want %d (clicked card)", m.selIdx, k)
	}
	keyFrame(KeyCodeNone, 0, m.rootView) // focus settles
	if !IdHasFocus(m.gridID) {
		t.Error("card click did not move keyboard focus to the grid wrapper")
	}
}

// TestGrid_FocusedArrowsConsumed: with the grid focused, each arrow keyframe
// moves the cursor exactly once (±1 across, ±cols up/down) — the focused
// handler consumes the key, so the global fallback cannot double-fire.
// Mirrors TestListFocusedArrowsMoveSelection.
func TestGrid_FocusedArrowsConsumed(t *testing.T) {
	sess, _ := seedGridSession(t, 8) // enough rows that no assertion clamps
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	focusGrid(t, m)
	if m.selIdx != 0 {
		t.Fatalf("initial selection index %d, want 0", m.selIdx)
	}
	keyFrame(KeyRight, 0, m.rootView)
	if m.selIdx != 1 {
		t.Fatalf("after focused Right: selIdx %d, want exactly 1 (double-fire?)", m.selIdx)
	}
	keyFrame(KeyDown, 0, m.rootView)
	if want := 1 + m.cols; m.selIdx != want {
		t.Fatalf("after focused Down: selIdx %d, want exactly %d (+cols once, cols=%d)", m.selIdx, want, m.cols)
	}
	keyFrame(KeyUp, 0, m.rootView)
	if m.selIdx != 1 {
		t.Errorf("after focused Up: selIdx %d, want exactly 1", m.selIdx)
	}
	keyFrame(KeyLeft, 0, m.rootView)
	if m.selIdx != 0 {
		t.Errorf("after focused Left: selIdx %d, want exactly 0", m.selIdx)
	}
}

// TestGrid_FocusedEnterOpensDetailForCursor: with the grid focused, Enter
// opens the detail panel for the CURSOR card exactly once per keyframe —
// open, then closed (consumed, so the global fallback cannot re-toggle).
// Mirrors TestListFocusedEnterTogglesDetailConsumed.
func TestGrid_FocusedEnterOpensDetailForCursor(t *testing.T) {
	sess, rows := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	focusGrid(t, m)
	keyFrame(KeyRight, 0, m.rootView) // cursor onto card 1
	if m.selIdx != 1 {
		t.Fatalf("cursor selIdx %d, want 1", m.selIdx)
	}
	keyFrame(KeyEnter, 0, m.rootView)
	if got := sess.Snapshot().Selected; got != rows[1].InstallDir {
		t.Fatalf("after focused Enter: selected %q, want %q (panel opens for the cursor card)", got, rows[1].InstallDir)
	}
	keyFrame(KeyEnter, 0, m.rootView)
	if got := sess.Snapshot().Selected; got != "" {
		t.Errorf("after second focused Enter: selected %q, want closed (toggled exactly once)", got)
	}
}

// TestGrid_FocusSurvivesPanelOpen: a card click opens the detail panel,
// which re-nests the grid wrapper (shirei identities are path-scoped, so
// the mid-click focus grab is orphaned); the deferred gridFocusPending
// re-assert must land focus on the fresh wrapper identity.
func TestGrid_FocusSurvivesPanelOpen(t *testing.T) {
	sess, _ := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build; captures m.gridID
	keyFrame(KeyCodeNone, 0, m.rootView) // capture card rects
	if m.cardRect.Size[0] == 0 {
		t.Fatalf("card rect not recorded: %+v", m.cardRect)
	}
	clickRect(m.cardRect, m.rootView)
	if got := sess.Snapshot().Selected; got == "" {
		t.Fatal("card click did not open the detail panel")
	}
	keyFrame(KeyCodeNone, 0, m.rootView) // re-nested wrapper consumes gridFocusPending
	if !IdHasFocus(m.gridID) {
		t.Error("grid focus not retained after the detail panel re-nested the wrapper")
	}
}

// TestGrid_CursorRingDistinctFromHover: the cursor ring (seam:
// m.gridCursorRect) tracks the keyboard cursor card only — hovering another
// card keeps the hover treatment on that card without moving the ring, and
// session-selecting a card draws no selection chrome on the grid (the
// docked panel is the grid-mode selection indicator; cards have no selBg).
func TestGrid_CursorRingDistinctFromHover(t *testing.T) {
	sess, rows := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build; cursor on card 0
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects
	if m.gridCursorRect.Size[0] == 0 {
		t.Fatal("cursor card never recorded a ring rect")
	}
	if m.selIdx != 0 {
		t.Fatalf("initial cursor selIdx %d, want 0", m.selIdx)
	}

	// Hover the last card (not the cursor): hover chrome applies there
	// (accent, tracked via hoveredDir) while the ring stays on card 0.
	last := len(rows) - 1
	hoverRect := m.cardRect // last rendered card
	if hoverRect == m.gridCursorRect {
		t.Fatalf("last card rect equals cursor rect; need two distinct cards: %+v", hoverRect)
	}
	InputState.MousePoint = Vec2{hoverRect.Origin[0] + hoverRect.Size[0]/2, hoverRect.Origin[1] + hoverRect.Size[1]/2}
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.hoveredDir != rows[last].InstallDir {
		t.Errorf("hoveredDir %q, want %q (hovered non-cursor card keeps its hover treatment)", m.hoveredDir, rows[last].InstallDir)
	}
	if m.gridCursorRect == hoverRect {
		t.Error("cursor ring followed the hovered card; want it pinned to the cursor card")
	}
	InputState.MousePoint = Vec2{-50, -50}

	// Selecting a card opens the panel but must not move the cursor ring or
	// draw any selection band on the card itself.
	sess.Select(rows[1].InstallDir)
	keyFrame(KeyCodeNone, 0, m.rootView) // panel opens beside the grid
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.gridCursorRect.Size[0] == 0 {
		t.Error("cursor ring lost after selection opened the panel")
	}
	if m.selIdx != 0 {
		t.Errorf("session selection moved the cursor: selIdx %d, want 0 (selection != cursor in grid mode)", m.selIdx)
	}
}

// TestGrid_ScrollIntoView: moving the keyboard cursor past the visible
// chunks scrolls the virtualized grid so the cursor card renders on screen
// (mirrors TestListSelectionScrollsIntoView; grid cards are taller than a
// short viewport, so intersection — not full containment — proves the
// chunk scrolled into view).
func TestGrid_ScrollIntoView(t *testing.T) {
	sess, _ := seedGridSession(t, 10) // 11 rows: several chunks past the fold
	m := newModel(Config{Session: sess})

	headlessFrames(t, 700, 400)          // short, narrow window: few chunks visible
	keyFrame(KeyCodeNone, 0, m.rootView) // build; derives m.cols from live width
	cols := m.cols
	if cols < 1 {
		t.Fatalf("cols %d, want >= 1", cols)
	}
	for range 3 {
		keyFrame(KeyDown, 0, m.rootView) // unfocused global fallback: +cols each
	}
	if want := 3 * cols; m.selIdx != want {
		t.Fatalf("selIdx %d, want %d (3 x Down of +%d cols)", m.selIdx, want, cols)
	}
	RunFrameFn(m.rootView) // consume the scroll-into-view command
	RunFrameFn(m.rootView) // re-render the cursor card with real layout data
	r := m.gridCursorRect
	if r.Size[0] == 0 {
		t.Fatal("cursor card never rendered after scrolling past the viewport")
	}
	if r.Origin[1]+r.Size[1] < 0 || r.Origin[1] > 400 {
		t.Errorf("cursor card rect %+v outside the 400px window (not scrolled into view)", r)
	}
}
