package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// viewSwitchPopulated builds a session with one scanned game and its model,
// ready for headless frame driving (non-empty library: the view switch is
// live).
func viewSwitchPopulated(t *testing.T) (*ui.Session, *model) {
	t.Helper()
	sess, _ := guiFakes(t)
	scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	headlessFrames(t, 1100, 700)
	InputState.MousePoint = Vec2{-50, -50}
	return sess, m
}

// focusViewSwitch builds two frames and Tabs until the view switch's outer
// wrapper holds keyboard focus (the loop tolerates sibling focusables —
// Scan, Add Game, the search field, the sort trigger — without hard-coding
// a count). One Tab stop is the design: the binary grid/list choice is a
// single focus target, not one per segment.
func focusViewSwitch(t *testing.T, m *model, view FrameFn) {
	t.Helper()
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if m.viewSwitchID == nil {
		t.Fatal("view switch not rendered (viewSwitchID nil)")
	}
	for tabs := 1; tabs <= 10; tabs++ {
		keyFrame(KeyTab, 0, view)      // CycleFocusOnTab moves focus
		keyFrame(KeyCodeNone, 0, view) // focus change applies
		if IdHasFocus(m.viewSwitchID) {
			t.Logf("view switch focused after %d Tab(s)", tabs)
			return
		}
	}
	t.Fatalf("view switch never took focus via Tab (viewSwitchID %v)", m.viewSwitchID)
}

// TestViewSwitch_TabFocusesAndRings: the view-switch wrapper is a Tab stop
// (Focusable + CycleFocusOnTab) and draws its focus ring (seam:
// m.viewSwitchFocusRing, mirroring m.sortFocusRing) exactly while it holds
// keyboard focus.
func TestViewSwitch_TabFocusesAndRings(t *testing.T) {
	_, m := viewSwitchPopulated(t)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.viewSwitchFocusRing {
		t.Error("focus ring drawn while the view switch is not focused")
	}

	focusViewSwitch(t, m, m.rootView)
	if !m.viewSwitchFocusRing {
		t.Error("focus ring not drawn while the view switch holds keyboard focus")
	}

	// Tabbing away drops the ring: the seam tracks focus exactly.
	keyFrame(KeyTab, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if IdHasFocus(m.viewSwitchID) {
		t.Error("view switch still focused after Tabbing away")
	}
	if m.viewSwitchFocusRing {
		t.Error("focus ring still drawn after the view switch lost focus")
	}
	t.Log("Tab focused the view switch and the ring drew only while focused")
}

// TestViewSwitch_ClickFocuses: clicking a segment hands keyboard focus to
// the OUTER wrapper (FocusOnClick) AND still toggles the view — grabbing
// focus must not eat the segment's PressAction.
func TestViewSwitch_ClickFocuses(t *testing.T) {
	sess, m := viewSwitchPopulated(t)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.listSegRect.Size[0] == 0 {
		t.Fatalf("list segment rect not recorded: %+v", m.listSegRect)
	}
	clickRect(m.listSegRect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if !IdHasFocus(m.viewSwitchID) {
		t.Error("click did not focus the view-switch wrapper")
	}
	if got := sess.Snapshot().Mode; got != ui.ViewList {
		t.Errorf("segment click left mode %v, want ViewList: FocusOnClick must not eat the activation", got)
	}
	t.Log("click focused the wrapper and the segment still toggled the view")
}

// TestViewSwitch_EnterTogglesView: Enter on the focused wrapper flips
// grid→list exactly once (the key is consumed, so nothing downstream can
// re-trigger), and the wrapper keeps focus across the flip (the toolbar
// renders every frame at a stable path, so the id survives).
func TestViewSwitch_EnterTogglesView(t *testing.T) {
	sess, m := viewSwitchPopulated(t)
	focusViewSwitch(t, m, m.rootView)

	keyFrame(KeyEnter, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sess.Snapshot().Mode; got != ui.ViewList {
		t.Errorf("Enter on the focused view switch left mode %v, want ViewList", got)
	}
	if !IdHasFocus(m.viewSwitchID) {
		t.Error("view switch lost keyboard focus across the Enter toggle")
	}

	// A second Enter flips back — proving the first press toggled exactly
	// once (an unconsumed key would double-toggle back to grid above).
	keyFrame(KeyEnter, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sess.Snapshot().Mode; got != ui.ViewGrid {
		t.Errorf("second Enter left mode %v, want ViewGrid (single toggle per press)", got)
	}
	t.Log("Enter toggled the view once per press and focus survived")
}

// TestViewSwitch_SpaceTogglesView: Space on the focused wrapper flips
// grid→list with the same single-toggle, focus-retained semantics as Enter.
func TestViewSwitch_SpaceTogglesView(t *testing.T) {
	sess, m := viewSwitchPopulated(t)
	focusViewSwitch(t, m, m.rootView)

	keyFrame(KeySpace, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sess.Snapshot().Mode; got != ui.ViewList {
		t.Errorf("Space on the focused view switch left mode %v, want ViewList", got)
	}
	if !IdHasFocus(m.viewSwitchID) {
		t.Error("view switch lost keyboard focus across the Space toggle")
	}
	t.Log("Space toggled the view and focus survived")
}

// TestViewSwitch_DisabledIgnoresKeys: with an empty library the wrapper is
// still focusable (one consistent Tab stop) but Enter/Space are inert — the
// disabled guard on segment PressAction extends to keys.
func TestViewSwitch_DisabledIgnoresKeys(t *testing.T) {
	sess, _ := guiFakes(t) // no scan: empty library
	m := newModel(Config{Session: sess})
	headlessFrames(t, 1100, 700)
	InputState.MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if !m.libraryEmpty() {
		t.Fatal("setup: library must be empty")
	}

	focusViewSwitch(t, m, m.rootView)
	for _, key := range []KeyCode{KeyEnter, KeySpace} {
		keyFrame(key, 0, m.rootView)
		keyFrame(KeyCodeNone, 0, m.rootView)
		if got := sess.Snapshot().Mode; got != ui.ViewGrid {
			t.Errorf("%v on the disabled view switch changed mode to %v, want unchanged", key, got)
		}
	}
	t.Log("empty library: view switch inert to Enter/Space")
}
