package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
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

// focusCard settles one frame (recording the fresh cardIDs registry), then
// moves keyboard focus onto the card for dir directly and settles another
// frame so HasFocus is stable. It fails the test when the card never takes
// focus. Mirrors focusList, retargeted from the removed wrapper to the card.
func focusCard(t *testing.T, m *model, dir string) ContainerId {
	t.Helper()
	keyFrame(KeyCodeNone, 0, m.rootView) // build; captures m.cardIDs
	id := m.cardIDs[dir]
	if id == nil {
		t.Fatalf("card %q not in the id registry (rendered?)", dir)
	}
	FocusImmediateOn(id)
	keyFrame(KeyCodeNone, 0, m.rootView) // focus applies at frame start
	if !IdHasFocus(m.cardIDs[dir]) {
		t.Fatalf("card %q did not take focus", dir)
	}
	return m.cardIDs[dir]
}

// TestGrid_WrapperNotFocusable: the grid region itself is no Tab stop — the
// last toolbar focusable (the view switch) hands focus straight to the
// FIRST CARD on a single Tab. If a focusable grid wrapper still existed, it
// would sit between the two and the first Tab would land on it instead.
func TestGrid_WrapperNotFocusable(t *testing.T) {
	sess, rows := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	keyFrame(KeyCodeNone, 0, m.rootView) // build; registers focusables, captures ids
	if m.viewSwitchID == nil {
		t.Fatal("view switch id not captured")
	}
	FocusImmediateOn(m.viewSwitchID)
	keyFrame(KeyCodeNone, 0, m.rootView) // focus settles on the view switch
	if !IdHasFocus(m.viewSwitchID) {
		t.Fatal("view switch did not take focus")
	}
	keyFrame(KeyTab, 0, m.rootView)      // CycleFocusOnTab: next focusable
	keyFrame(KeyCodeNone, 0, m.rootView) // focus change applies
	cardID := m.cardIDs[rows[0].InstallDir]
	if cardID == nil {
		t.Fatalf("first card %q not in the id registry", rows[0].InstallDir)
	}
	if !IdHasFocus(cardID) {
		t.Error("one Tab from the view switch did not land on the first card; a grid container is still a Tab stop")
	}
}

// TestGrid_TabOrderCardThenInnerItems: Tab order inside the grid is the
// render order of shirei's focusables registry: card → its version-dropdown
// trigger → its buttons → the NEXT card. The card with the dropdown is an
// external install (OptiScaler pill = dropdown trigger); the buttons are
// asserted by elimination (no id seams on focusableButton wrappers).
func TestGrid_TabOrderCardThenInnerItems(t *testing.T) {
	sess, _ := guiFakes(t)
	var extDir string
	for i := range 2 {
		dir := filepath.Join(t.TempDir(), fmt.Sprintf("Game%02d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MZGAME"), 0o644); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			// A branded OptiScaler dll makes this manual game external, so
			// its card renders the version-dropdown trigger (manual games
			// probe the game dir itself — app.go's manual-root detect).
			markExternal(t, dir, [4]uint16{0, 7, 0, 0})
			extDir = dir
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
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build; captures registries
	keyFrame(KeyCodeNone, 0, m.rootView) // settle
	rows := m.visibleRows()
	k := -1
	for i, r := range rows {
		if r.InstallDir == extDir {
			k = i
		}
	}
	if k < 0 {
		t.Fatalf("external row %q not among visible rows %v", extDir, rows)
	}
	if rows[k].Status != domain.StatusExternal {
		t.Fatalf("row %q status %q, want external (the marker must render the dropdown)", extDir, rows[k].Status)
	}
	if k+1 >= len(rows) {
		t.Fatalf("external row is the last card (k=%d of %d); need a successor card for the order proof", k, len(rows))
	}
	cardK := focusCard(t, m, rows[k].InstallDir)
	cardNext := m.cardIDs[rows[k+1].InstallDir]
	ddK := m.cardDDTrigger[rows[k].InstallDir]
	if cardNext == nil {
		t.Fatalf("successor card %q not in the id registry", rows[k+1].InstallDir)
	}
	if ddK == nil {
		t.Fatalf("external card %q rendered no version-dropdown trigger", rows[k].InstallDir)
	}
	tab := func() {
		keyFrame(KeyTab, 0, m.rootView)      // CycleFocusOnTab moves focus
		keyFrame(KeyCodeNone, 0, m.rootView) // focus change applies
	}
	// Tab 1: card → its version-dropdown trigger.
	tab()
	if !IdHasFocus(ddK) {
		t.Error("Tab from a focused card did not land on its version-dropdown trigger")
	}
	if IdHasFocus(cardK) || IdHasFocus(cardNext) {
		t.Error("Tab from a focused card landed on a card; want the dropdown trigger first")
	}
	// Tabs 2 and 3: the card's two buttons (Adopt, Launch) — no id seams, so
	// assert by elimination: focus is on neither card nor the trigger.
	for i := range 2 {
		tab()
		if IdHasFocus(cardK) || IdHasFocus(cardNext) || IdHasFocus(ddK) {
			t.Errorf("inner-item Tab %d left the card's inner items; want its buttons", i+2)
		}
	}
	// Tab 4: the buttons are exhausted — focus lands on the NEXT card, and
	// the cursor follows focus (ReceivedFocusNow sync).
	tab()
	if !IdHasFocus(cardNext) {
		t.Error("Tab past a card's inner items did not land on the next card")
	}
	if m.selIdx != k+1 {
		t.Errorf("cursor selIdx %d after Tabbing onto card %d; cursor must follow focus", m.selIdx, k+1)
	}
}

// TestGrid_CardClickFocusesSelectsAndMovesCursor: clicking a card body
// selects the game (detail panel), moves the keyboard cursor onto the
// clicked card, and puts keyboard focus on THE CARD — exclusively, not a
// wrapper, not an inner control. Mirrors TestListRowClickFocusesAndSelects.
func TestGrid_CardClickFocusesSelectsAndMovesCursor(t *testing.T) {
	sess, rows := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	// Wide window: all cards land in one chunk, so the last rendered card
	// (m.cardRect is overwritten per card) is the last row.
	headlessFrames(t, 1200, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build; captures m.cardIDs
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
	keyFrame(KeyCodeNone, 0, m.rootView) // panel re-nests; deferred re-assert lands
	if !IdHasFocus(m.cardIDs[rows[k].InstallDir]) {
		t.Error("card click did not move keyboard focus to the card itself")
	}
}

// TestGrid_CardClickExclusive: after clicking a card, NOTHING else holds
// keyboard focus — not the search field, not a toolbar control, not another
// card, not an inner control of the clicked card.
func TestGrid_CardClickExclusive(t *testing.T) {
	sess, rows := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build; captures registries
	keyFrame(KeyCodeNone, 0, m.rootView) // capture card rects
	k := len(rows) - 1
	clickRect(m.cardRect, m.rootView) // last rendered card = last row
	keyFrame(KeyCodeNone, 0, m.rootView)

	clicked := m.cardIDs[rows[k].InstallDir]
	if !IdHasFocus(clicked) {
		t.Fatal("clicked card does not hold focus")
	}
	focused := 0
	for dir, id := range m.cardIDs {
		if IdHasFocus(id) {
			focused++
			if dir != rows[k].InstallDir {
				t.Errorf("card %q holds focus alongside the clicked card %q", dir, rows[k].InstallDir)
			}
		}
	}
	if focused != 1 {
		t.Errorf("%d cards hold focus after a card click, want exactly 1", focused)
	}
	for name, id := range map[string]ContainerId{
		"search":      m.searchID,
		"sort":        m.sortTriggerID,
		"view switch": m.viewSwitchID,
	} {
		if id != nil && IdHasFocus(id) {
			t.Errorf("%s holds focus after a card click; want the card exclusively", name)
		}
	}
}

// TestGrid_FocusedEnterOpensDetail: Enter on a focused card opens the detail
// panel for THAT card exactly once per keyframe (consumed, so the global
// fallback cannot re-toggle); a second Enter closes it.
func TestGrid_FocusedEnterOpensDetail(t *testing.T) {
	sess, rows := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	focusCard(t, m, rows[1].InstallDir)
	if m.selIdx != 1 {
		m.selIdx = 1 // FocusImmediateOn bypasses ReceivedFocusNow; pin the cursor
	}
	keyFrame(KeyEnter, 0, m.rootView)
	if got := sess.Snapshot().Selected; got != rows[1].InstallDir {
		t.Fatalf("after focused Enter: selected %q, want %q (panel opens for the focused card)", got, rows[1].InstallDir)
	}
	keyFrame(KeyCodeNone, 0, m.rootView) // panel re-nests; deferred re-assert lands
	if !IdHasFocus(m.cardIDs[rows[1].InstallDir]) {
		t.Fatal("card lost focus when its Enter opened the detail panel")
	}
	keyFrame(KeyEnter, 0, m.rootView)
	if got := sess.Snapshot().Selected; got != "" {
		t.Errorf("after second focused Enter: selected %q, want closed (toggled exactly once)", got)
	}
}

// TestGrid_FocusedArrowsMoveFocusAndCursor: with a card focused, each arrow
// keyframe moves the cursor exactly once (±1 across, ±cols up/down — the key
// is consumed, so the global fallback cannot double-fire) AND hands focus to
// the new cursor card: focus and cursor never diverge.
func TestGrid_FocusedArrowsMoveFocusAndCursor(t *testing.T) {
	sess, rows := seedGridSession(t, 8) // enough rows that no assertion clamps
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	focusCard(t, m, rows[0].InstallDir)
	m.selIdx = 0 // FocusImmediateOn bypasses ReceivedFocusNow; pin the cursor
	assertFocus := func(want int, what string) {
		t.Helper()
		if m.selIdx != want {
			t.Fatalf("%s: selIdx %d, want exactly %d (double-fire?)", what, m.selIdx, want)
		}
		if !IdHasFocus(m.cardIDs[rows[want].InstallDir]) {
			t.Fatalf("%s: focus did not follow the cursor to card %d", what, want)
		}
	}
	keyFrame(KeyRight, 0, m.rootView)
	assertFocus(1, "after focused Right")
	keyFrame(KeyDown, 0, m.rootView)
	assertFocus(1+m.cols, "after focused Down")
	keyFrame(KeyUp, 0, m.rootView)
	assertFocus(1, "after focused Up")
	keyFrame(KeyLeft, 0, m.rootView)
	assertFocus(0, "after focused Left")
}

// TestGrid_FocusSurvivesPanelOpen: a card click opens the detail panel,
// which re-nests the grid (shirei identities are path-scoped, so the
// mid-click focus grab is orphaned); the deferred cardFocusPending
// re-assert must land focus on the card's fresh identity.
func TestGrid_FocusSurvivesPanelOpen(t *testing.T) {
	sess, rows := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build; captures m.cardIDs
	keyFrame(KeyCodeNone, 0, m.rootView) // capture card rects
	if m.cardRect.Size[0] == 0 {
		t.Fatalf("card rect not recorded: %+v", m.cardRect)
	}
	k := len(rows) - 1 // last rendered card = last row
	clickRect(m.cardRect, m.rootView)
	if got := sess.Snapshot().Selected; got == "" {
		t.Fatal("card click did not open the detail panel")
	}
	keyFrame(KeyCodeNone, 0, m.rootView) // re-nested grid consumes cardFocusPending
	if !IdHasFocus(m.cardIDs[rows[k].InstallDir]) {
		t.Error("card focus not retained after the detail panel re-nested the grid")
	}
}

// TestGrid_RingFullBounds: the focused card's focus ring must render on all
// four sides — the ring rect equals the card's FULL bounds, and the card
// keeps at least 1px of clearance inside its clipping chunk row on every
// side so shirei's edge-straddling border stroke (half outside the rect) is
// never scissored (the thin top/bottom ring bug).
func TestGrid_RingFullBounds(t *testing.T) {
	sess, rows := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	focusCard(t, m, rows[len(rows)-1].InstallDir)
	ring := m.gridCursorRect
	if ring.Size[0] == 0 {
		t.Fatal("focused card never recorded a ring rect")
	}
	// The last rendered card is the last row, so m.cardRect is the same
	// card: the ring must cover the card's full bounds, not a clipped subset.
	if ring != m.cardRect {
		t.Errorf("ring rect %+v != card rect %+v; the ring must cover the full card bounds", ring, m.cardRect)
	}
	clip := m.cardRingClip
	if clip.Size[0] == 0 {
		t.Fatal("ringed card never recorded its chunk-row clip rect")
	}
	if got := ring.Origin[1] - clip.Origin[1]; got < 1 {
		t.Errorf("top ring clearance %vpx, want >= 1 (the outer half-stroke is clipped)", got)
	}
	if got := (clip.Origin[1] + clip.Size[1]) - (ring.Origin[1] + ring.Size[1]); got < 1 {
		t.Errorf("bottom ring clearance %vpx, want >= 1 (the outer half-stroke is clipped)", got)
	}
	if got := ring.Origin[0] - clip.Origin[0]; got < 1 {
		t.Errorf("left ring clearance %vpx, want >= 1", got)
	}
	if got := (clip.Origin[0] + clip.Size[0]) - (ring.Origin[0] + ring.Size[0]); got < 1 {
		t.Errorf("right ring clearance %vpx, want >= 1", got)
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
