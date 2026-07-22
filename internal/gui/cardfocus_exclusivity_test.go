package gui

import (
	"testing"

	. "go.hasen.dev/shirei"
)

// Card-focus exclusivity discriminating tests. User report: "changing focus
// to a card still leaves the previous card's buttons focused — when a card
// takes focus (Tab or click), no other element must have it." Each test
// targets ONE candidate mechanism so a failure names the culprit:
//
//	H(a) stale-hover click: a fast move+click onto another card's body.
//	H(b) Tab walk: an inner control of card A holding focus must not leave
//	     card A's selIdx cursor ring on (the one-ring rule tracks cards only).
//	H(c) arrows with a button focused: the global arrow handler moves the
//	     cursor but never releases the inner control's focus.

// tabFocus runs one Tab cycle (cycle frame + apply frame).
func tabFocus(m *model) {
	keyFrame(KeyTab, 0, m.rootView)      // CycleFocusOnTab moves focus
	keyFrame(KeyCodeNone, 0, m.rootView) // focus change applies
}

// focusCardButton leaves card dir's first inner focusable (its quick-action
// button — seeded games render no version dropdown) holding keyboard focus,
// failing the test if focus never leaves the cards. Setup only: how the
// button got focused is not the mechanism under test (the click/arrow that
// FOLLOWS is); Tabbing onto it is deterministic, unlike clicking the button
// row's center, which can land in the inter-button gap.
func focusCardButton(t *testing.T, m *model, dir string) {
	t.Helper()
	focusCard(t, m, dir)
	tabFocus(m)
	for dir, id := range m.cardIDs {
		if IdHasFocus(id) {
			t.Fatalf("Tab from card %q kept focus on card %q; want its first inner control", dir, dir)
		}
	}
}

// TestCardFocus_ClickOtherCardBlursButton (H(a): STALE-HOVER CLICK): with
// card A's quick-action button focused, a FAST move+click (no hover-settle
// frame) onto card B's body must focus card B and release the button.
// Theory under test: hoverList is computed at frame start from the previous
// frame's artifacts, so a fast move+click could leave the old button
// IsHovered on the click frame — its FocusOnClick blur branch would never
// fire and the new card's hover-gated FocusOnClick would never grab.
func TestCardFocus_ClickOtherCardBlursButton(t *testing.T) {
	sess, rows := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	focusCardButton(t, m, rows[0].InstallDir)
	m.selIdx = 0

	// Card B is the last rendered card (m.cardRect is overwritten per
	// card), so its rect is current after the frames above.
	b := len(rows) - 1
	if m.cardRect.Size[0] == 0 {
		t.Fatalf("card rect not recorded: %+v", m.cardRect)
	}
	// Fast move+click: the pointer jumps onto card B and goes down in the
	// SAME frame — no hover-settle frame first (clickRect minus its first
	// frame). This is the gesture the stale-hover theory is about.
	InputState.MousePoint = Vec2{m.cardRect.Origin[0] + m.cardRect.Size[0]/2, m.cardRect.Origin[1] + m.cardRect.Size[1]/2}
	FrameInput.Mouse = MouseClick
	RunFrameFn(m.rootView)
	FrameInput.Mouse = MouseRelease
	RunFrameFn(m.rootView)
	FrameInput.Mouse = 0
	keyFrame(KeyCodeNone, 0, m.rootView) // panel re-nests; deferred re-assert lands

	cardB := m.cardIDs[rows[b].InstallDir]
	if !IdHasFocus(cardB) {
		t.Error("fast move+click onto card B did not focus it; card A's button kept focus (stale-hover click bug)")
	}
	for dir, id := range m.cardIDs {
		if dir != rows[b].InstallDir && IdHasFocus(id) {
			t.Errorf("card %q holds focus alongside clicked card %q; want exactly one focused element", dir, rows[b].InstallDir)
		}
	}
}

// TestCardFocus_TabLeavesNoButtonRingOnPriorCard (H(b): TAB WALK): Tabbing
// from card A onto its inner controls must not leave card A's selIdx cursor
// ring lit alongside the focused control's own ring — the one-ring rule
// (gridCardFocused) only tracks CARDS, so a focused inner control leaves
// the cursor ring on: the double ring. Tabbing on to card B must leave
// nothing of card A focused and exactly one ring.
func TestCardFocus_TabLeavesNoButtonRingOnPriorCard(t *testing.T) {
	sess, rows := seedGridSession(t, 3)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	focusCardButton(t, m, rows[0].InstallDir)
	m.selIdx = 0

	// Card A's button holds focus while the cursor still sits on card A:
	// the cursor ring must be suppressed (the focused control wears the
	// only ring). gridCursorRect is the ring seam; reset it and render one
	// frame — any ring drawn re-records it.
	m.gridCursorRect = Rect{}
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.gridCursorRect.Size[0] != 0 {
		t.Errorf("cursor ring drawn on card A (rect %+v) while its button holds focus; want it suppressed (double ring)", m.gridCursorRect)
	}

	// Keep Tabbing until card B's container holds focus (button count
	// varies: quick-action always, Launch when launchable).
	cardB := m.cardIDs[rows[1].InstallDir]
	for range 3 {
		if IdHasFocus(cardB) {
			break
		}
		tabFocus(m)
	}
	if !IdHasFocus(cardB) {
		t.Fatal("Tabbing past card A's inner controls never landed on card B")
	}
	if IdHasFocus(m.cardIDs[rows[0].InstallDir]) {
		t.Error("card A still holds focus after focus reached card B")
	}
	if m.selIdx != 1 {
		t.Errorf("cursor selIdx %d after Tabbing onto card B, want 1 (cursor follows focus)", m.selIdx)
	}
	// Exactly one ring: the focused card B wears it; no cursor ring may
	// appear on any other card. The seam records the LAST ring drawn per
	// frame, so a stray cursor ring on a later-rendered card would
	// overwrite it — assert it stays card B's rect by rendering once more
	// and checking the recorded ring did not move off card B.
	ring := m.gridCursorRect
	if ring.Size[0] == 0 {
		t.Error("focused card B recorded no ring; want exactly one (its own)")
	}
	m.gridCursorRect = Rect{}
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.gridCursorRect != ring {
		t.Errorf("ring moved between frames with card B focused: %+v -> %+v; want exactly one stable ring", ring, m.gridCursorRect)
	}
}

// TestCardFocus_ArrowsWithButtonFocusedMoveFocus (H(c): ARROWS WITH BUTTON
// FOCUSED): with card A's quick-action button focused, pressing Right must
// move the cursor to card B AND release the button's focus (user model:
// moving to a card means no other element holds focus). The probe for
// "button released": a following Enter must reach the GLOBAL handler and
// toggle the detail panel for the cursor card — a still-focused button
// would consume Enter and re-activate instead.
func TestCardFocus_ArrowsWithButtonFocusedMoveFocus(t *testing.T) {
	sess, rows := seedGridSession(t, 8) // enough rows that no assertion clamps
	m := newModel(Config{Session: sess})

	headlessFrames(t, 800, 600)
	focusCardButton(t, m, rows[0].InstallDir)
	m.selIdx = 0

	keyFrame(KeyRight, 0, m.rootView)
	if m.selIdx != 1 {
		t.Fatalf("Right with card A's button focused: selIdx %d, want 1 (cursor must move exactly once)", m.selIdx)
	}
	// Focus/cursor divergence check: the cursor card must not still be
	// sharing the screen with a focused inner control of card A. The
	// observable proof is the Enter probe below.
	keyFrame(KeyEnter, 0, m.rootView)
	if got := sess.Snapshot().Selected; got != rows[1].InstallDir {
		t.Errorf("Enter after the cursor moved: Selected %q, want %q — card A's button kept focus and consumed Enter", got, rows[1].InstallDir)
	}
}
