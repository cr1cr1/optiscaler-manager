package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// sortPopulated builds a session with one scanned game and its model, ready
// for headless frame driving (non-empty library: the sort trigger is live).
func sortPopulated(t *testing.T) (*ui.Session, *model, ui.GameRow) {
	t.Helper()
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	headlessFrames(t, 1100, 700)
	InputState.MousePoint = Vec2{-50, -50}
	closeSortDropdownIfOpen(t, m, m.rootView)
	return sess, m, row
}

// closeSortDropdownIfOpen settles any dropdown state inherited through the
// process-wide identity tree: a Use-hook slot touched on the previous test's
// last frame still reads as retained on this test's first frame, so an
// earlier test that ended with the dropdown open would leak that open flag
// into this one. One probe frame surfaces the leak (the popup rebuilds the
// items seam), then Esc — consumed by the open dropdown — closes it.
func closeSortDropdownIfOpen(t *testing.T, m *model, view FrameFn) {
	t.Helper()
	keyFrame(KeyCodeNone, 0, view)
	if len(m.sortMenuItems) == 0 {
		return
	}
	keyFrame(KeyEscape, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if len(m.sortMenuItems) != 0 {
		t.Fatalf("inherited sort dropdown did not close on Esc (items %d)", len(m.sortMenuItems))
	}
}

// focusSortTrigger builds two frames and Tabs until the toolbar's sort
// trigger holds keyboard focus (the loop tolerates sibling focusables —
// Scan, Add Game, the search field — without hard-coding a count).
func focusSortTrigger(t *testing.T, m *model, view FrameFn) {
	t.Helper()
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if m.sortTriggerID == nil {
		t.Fatal("sort dropdown trigger not rendered (sortTriggerID nil)")
	}
	for tabs := 1; tabs <= 8; tabs++ {
		keyFrame(KeyTab, 0, view)      // CycleFocusOnTab moves focus
		keyFrame(KeyCodeNone, 0, view) // focus change applies
		if IdHasFocus(m.sortTriggerID) {
			t.Logf("sort trigger focused after %d Tab(s)", tabs)
			return
		}
	}
	t.Fatalf("sort trigger never took focus via Tab (sortTriggerID %v)", m.sortTriggerID)
}

// openSortDropdown clicks the toolbar sort trigger and settles one frame so
// the popup's item rects resolve.
func openSortDropdown(t *testing.T, m *model, view FrameFn) {
	t.Helper()
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if m.sortTriggerID == nil {
		t.Fatal("sort dropdown trigger not rendered (sortTriggerID nil)")
	}
	r := GetScreenRectOf(m.sortTriggerID)
	if r.Size[0] == 0 || r.Size[1] == 0 {
		t.Fatalf("sort trigger rect unresolved: %+v", r)
	}
	clickRect(r, view)
	keyFrame(KeyCodeNone, 0, view) // popup item rects resolve from the open frame
	if len(m.sortMenuItems) != 2 {
		t.Fatalf("sort dropdown did not open with its 2 items (got %d)", len(m.sortMenuItems))
	}
}

// focusOpenSortDropdown Tabs to the sort trigger and opens the dropdown
// with Enter, so arrow-key tests start from keyboard focus on the trigger
// with the popup open and the highlight initialized.
func focusOpenSortDropdown(t *testing.T, m *model, view FrameFn) {
	t.Helper()
	focusSortTrigger(t, m, view)
	keyFrame(KeyEnter, 0, view)
	keyFrame(KeyCodeNone, 0, view) // popup frame settles, highlight initializes
	if len(m.sortMenuItems) != 2 {
		t.Fatalf("Enter on the focused trigger did not open the dropdown (items %d, want 2)", len(m.sortMenuItems))
	}
	if !IdHasFocus(m.sortTriggerID) {
		t.Fatal("trigger lost keyboard focus on open")
	}
}

// sortHlIndex returns the highlighted sort-menu row index, requiring
// exactly one highlighted row.
func sortHlIndex(t *testing.T, m *model) int {
	t.Helper()
	found := -1
	for i, it := range m.sortMenuItems {
		if it.hl {
			if found >= 0 {
				t.Fatalf("multiple highlighted sort rows (%d and %d)", found, i)
			}
			found = i
		}
	}
	if found < 0 {
		t.Fatalf("no highlighted sort row: %+v", m.sortMenuItems)
	}
	return found
}

// TestSortDropdown_InitialHighlightIsCurrentMode: opening the dropdown
// initializes the highlight on the CURRENT sort mode's row — row 0 while
// SortDefault is active, and the "Name (A–Z)" row after switching to
// SortName and reopening.
func TestSortDropdown_InitialHighlightIsCurrentMode(t *testing.T) {
	sess, m, _ := sortPopulated(t)
	focusOpenSortDropdown(t, m, m.rootView)
	if got := sortHlIndex(t, m); got != 0 {
		t.Errorf("initial highlight row %d, want 0 (current mode is SortDefault)", got)
	}

	// Switch to SortName via a mouse pick, then reopen: the highlight must
	// initialize on the NEW current mode's row.
	target := -1
	for i, it := range m.sortMenuItems {
		if it.label == "Name (A–Z)" {
			target = i
		}
	}
	if target < 0 {
		t.Fatalf("no \"Name (A–Z)\" item offered: %+v", m.sortMenuItems)
	}
	clickRect(m.sortMenuItems[target].rect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	m.drain()
	if got := sess.Snapshot().Sort; got != ui.SortName {
		t.Fatalf("sort after pick = %v, want %v", got, ui.SortName)
	}
	if m.state.Sort != ui.SortName {
		t.Fatalf("model sort state %v, want %v after drain", m.state.Sort, ui.SortName)
	}
	if !IdHasFocus(m.sortTriggerID) {
		t.Fatal("trigger lost focus after the pick; cannot reopen with Enter")
	}
	// Park the mouse off the popup so hover cannot move the highlight and
	// mask a missing open-time init.
	InputState.MousePoint = Vec2{-50, -50}
	keyFrame(KeyEnter, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if len(m.sortMenuItems) != 2 {
		t.Fatalf("reopen failed (items %d, want 2)", len(m.sortMenuItems))
	}
	if got := sortHlIndex(t, m); got != target {
		t.Errorf("highlight after reopen = row %d, want %d (current mode is SortName)", got, target)
	}
	t.Log("highlight initializes on the current sort mode at each open")
}

// TestSortDropdown_ArrowNavigatesHighlight: with the dropdown open and the
// trigger focused, Down/Up move the highlight one row at a time without
// dispatching anything.
func TestSortDropdown_ArrowNavigatesHighlight(t *testing.T) {
	sess, m, _ := sortPopulated(t)
	focusOpenSortDropdown(t, m, m.rootView)
	if got := sortHlIndex(t, m); got != 0 {
		t.Fatalf("initial highlight row %d, want 0", got)
	}

	keyFrame(KeyDown, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sortHlIndex(t, m); got != 1 {
		t.Errorf("highlight after Down = row %d, want 1", got)
	}
	keyFrame(KeyUp, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sortHlIndex(t, m); got != 0 {
		t.Errorf("highlight after Up = row %d, want 0", got)
	}
	if got := sess.Snapshot().Sort; got != ui.SortDefault {
		t.Errorf("arrow navigation changed the sort to %v, want unchanged (no dispatch)", got)
	}
	if len(m.sortMenuItems) != 2 {
		t.Errorf("arrow navigation closed the dropdown (items %d, want 2)", len(m.sortMenuItems))
	}
	t.Log("Down/Up moved the highlight one row at a time; nothing dispatched")
}

// TestSortDropdown_HighlightWraps: Up from the first row wraps to the last,
// Down from the last row wraps to the first (no clamping).
func TestSortDropdown_HighlightWraps(t *testing.T) {
	_, m, _ := sortPopulated(t)
	focusOpenSortDropdown(t, m, m.rootView)
	if got := sortHlIndex(t, m); got != 0 {
		t.Fatalf("initial highlight row %d, want 0", got)
	}

	keyFrame(KeyUp, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sortHlIndex(t, m); got != 1 {
		t.Errorf("Up from row 0 = row %d, want 1 (wrap to the last row)", got)
	}
	keyFrame(KeyDown, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sortHlIndex(t, m); got != 0 {
		t.Errorf("Down from the last row = row %d, want 0 (wrap to the first row)", got)
	}
	t.Log("highlight wraps around both ends")
}

// TestSortDropdown_HoverMovesHighlight: hovering a row with the mouse
// adopts it as the highlight (input modes never fight), and the highlight
// sticks when the mouse leaves.
func TestSortDropdown_HoverMovesHighlight(t *testing.T) {
	_, m, _ := sortPopulated(t)
	focusOpenSortDropdown(t, m, m.rootView)
	keyFrame(KeyDown, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sortHlIndex(t, m); got != 1 {
		t.Fatalf("setup: highlight after Down = row %d, want 1", got)
	}

	// Hover row 0 (no click): the highlight must follow the mouse.
	r := m.sortMenuItems[0].rect
	if r.Size[0] == 0 {
		t.Fatalf("menu item rect unresolved: %+v", r)
	}
	InputState.MousePoint = Vec2{r.Origin[0] + r.Size[0]/2, r.Origin[1] + r.Size[1]/2}
	keyFrame(KeyCodeNone, 0, m.rootView) // hover settles
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sortHlIndex(t, m); got != 0 {
		t.Errorf("highlight after hovering row 0 = row %d, want 0 (mouse must move the highlight)", got)
	}

	// Mouse leaves: the last highlight position sticks (no snap-back).
	InputState.MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sortHlIndex(t, m); got != 0 {
		t.Errorf("highlight after the mouse left = row %d, want 0 (last position sticks)", got)
	}
	t.Log("hover moved the highlight; keyboard and mouse stay in sync")
}

// TestSortDropdown_EnterActivatesHighlighted: Enter on the focused trigger
// with the dropdown open activates the highlighted row exactly like a
// click pick: setSort fires, the menu closes, and focus returns to the
// trigger.
func TestSortDropdown_EnterActivatesHighlighted(t *testing.T) {
	sess, m, _ := sortPopulated(t)
	focusOpenSortDropdown(t, m, m.rootView)
	keyFrame(KeyDown, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sortHlIndex(t, m); got != 1 {
		t.Fatalf("setup: highlight after Down = row %d, want 1 (\"Name (A–Z)\")", got)
	}

	keyFrame(KeyEnter, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sess.Snapshot().Sort; got != ui.SortName {
		t.Errorf("sort after Enter = %v, want %v (Enter must activate the highlighted row)", got, ui.SortName)
	}
	if len(m.sortMenuItems) != 0 {
		t.Errorf("dropdown still open after Enter (items %d, want 0)", len(m.sortMenuItems))
	}
	if !IdHasFocus(m.sortTriggerID) {
		t.Error("trigger lost keyboard focus after the Enter pick")
	}
	t.Log("Enter activated the highlighted row, closed, and returned focus")
}

// TestSortDropdown_HighlightResetsOnReopen: the highlight re-initializes on
// the current mode's row at each open — a highlight moved with the arrows
// and dismissed with Esc does not leak into the next open.
func TestSortDropdown_HighlightResetsOnReopen(t *testing.T) {
	_, m, _ := sortPopulated(t)
	focusOpenSortDropdown(t, m, m.rootView)
	keyFrame(KeyDown, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sortHlIndex(t, m); got != 1 {
		t.Fatalf("setup: highlight after Down = row %d, want 1", got)
	}

	keyFrame(KeyEscape, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if len(m.sortMenuItems) != 0 {
		t.Fatalf("Esc did not close the dropdown (items %d)", len(m.sortMenuItems))
	}

	keyFrame(KeyEnter, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if len(m.sortMenuItems) != 2 {
		t.Fatalf("reopen failed (items %d, want 2)", len(m.sortMenuItems))
	}
	if got := sortHlIndex(t, m); got != 0 {
		t.Errorf("highlight after reopen = row %d, want 0 (re-initialized on the current mode)", got)
	}
	t.Log("highlight re-initializes on each open")
}

// TestSortDropdown_TriggerRendered: the toolbar renders the sort trigger
// (seam set, rect resolved) and the closed menu exposes no items.
func TestSortDropdown_TriggerRendered(t *testing.T) {
	_, m, _ := sortPopulated(t)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.sortTriggerID == nil {
		t.Fatal("sort dropdown trigger not rendered (sortTriggerID nil)")
	}
	r := GetScreenRectOf(m.sortTriggerID)
	if r.Size[0] == 0 || r.Size[1] == 0 {
		t.Errorf("sort trigger rect unresolved: %+v", r)
	}
	if len(m.sortMenuItems) != 0 {
		t.Errorf("closed sort dropdown exposes %d menu items, want 0", len(m.sortMenuItems))
	}
	if m.sortFocusRing {
		t.Error("focus ring drawn while the sort trigger is not focused")
	}
	t.Logf("sort trigger rect: %+v; menu closed, no items", r)
}

// TestSortDropdown_TabFocusRing: the trigger is a Tab stop (Focusable +
// CycleFocusOnTab) and draws its focus-ring branch (focusBorder) exactly
// while it holds keyboard focus (seam: m.sortFocusRing, mirroring
// m.ddFocusRing).
func TestSortDropdown_TabFocusRing(t *testing.T) {
	_, m, _ := sortPopulated(t)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.sortFocusRing {
		t.Error("focus ring drawn while the trigger is not focused")
	}

	focusSortTrigger(t, m, m.rootView)
	if !m.sortFocusRing {
		t.Error("focus ring not drawn while the trigger holds keyboard focus")
	}
	t.Log("Tab focused the sort trigger and the focus ring drew only while focused")
}

// TestSortDropdown_ClickFocuses: clicking the trigger hands it keyboard
// focus (FocusOnClick) AND still opens the popup — grabbing focus must not
// eat the click's activation.
func TestSortDropdown_ClickFocuses(t *testing.T) {
	_, m, _ := sortPopulated(t)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	r := GetScreenRectOf(m.sortTriggerID)
	if r.Size[0] == 0 || r.Size[1] == 0 {
		t.Fatalf("sort trigger rect unresolved: %+v", r)
	}
	clickRect(r, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView) // popup frame settles
	if !IdHasFocus(m.sortTriggerID) {
		t.Error("click did not focus the sort trigger")
	}
	if len(m.sortMenuItems) != 2 {
		t.Errorf("click did not open the dropdown (items %d, want 2): FocusOnClick must not eat the activation", len(m.sortMenuItems))
	}
	t.Log("click focused the trigger and opened the dropdown")
}

// TestSortDropdown_EnterSpaceToggle: Enter and Space on the FOCUSED trigger
// each toggle the dropdown open (two items: "Default (actionable first)",
// "Name (A–Z)") and closed, dispatching nothing.
func TestSortDropdown_EnterSpaceToggle(t *testing.T) {
	sess, m, _ := sortPopulated(t)
	focusSortTrigger(t, m, m.rootView)

	toggle := func(key KeyCode) {
		keyFrame(key, 0, m.rootView)
		keyFrame(KeyCodeNone, 0, m.rootView) // popup open/close settles
	}
	labelsChecked := false
	for _, key := range []KeyCode{KeyEnter, KeySpace} {
		toggle(key)
		if len(m.sortMenuItems) != 2 {
			t.Fatalf("%v on the focused trigger did not open the dropdown (items %d, want 2)", key, len(m.sortMenuItems))
		}
		if !labelsChecked {
			labelsChecked = true
			if m.sortMenuItems[0].label != "Default (actionable first)" {
				t.Errorf("item 0 label %q, want %q", m.sortMenuItems[0].label, "Default (actionable first)")
			}
			if m.sortMenuItems[1].label != "Name (A–Z)" {
				t.Errorf("item 1 label %q, want %q", m.sortMenuItems[1].label, "Name (A–Z)")
			}
		}
		toggle(key)
		if len(m.sortMenuItems) != 0 {
			t.Errorf("second %v on the focused trigger did not close the dropdown (items %d, want 0)", key, len(m.sortMenuItems))
		}
	}
	if got := sess.Snapshot().Sort; got != ui.SortDefault {
		t.Errorf("keyboard toggling changed the sort to %v, want unchanged (no dispatch)", got)
	}
	if !IdHasFocus(m.sortTriggerID) {
		t.Error("trigger lost keyboard focus across the open/close toggles")
	}
	t.Log("Enter and Space each toggled the dropdown open and closed")
}

// TestSortDropdown_MenuPickSetsSort: clicking the "Name (A–Z)" item drives
// setSort(ui.SortName), closes the popup, and keeps focus on the trigger
// (the toolbar's stable identity makes the re-assert immediate).
func TestSortDropdown_MenuPickSetsSort(t *testing.T) {
	sess, m, _ := sortPopulated(t)
	openSortDropdown(t, m, m.rootView)

	target := -1
	for i, it := range m.sortMenuItems {
		if it.label == "Name (A–Z)" {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("no \"Name (A–Z)\" item offered: %+v", m.sortMenuItems)
	}
	picked := m.sortMenuItems[target]
	if picked.rect.Size[0] == 0 {
		t.Fatalf("menu item rect unresolved: %+v", picked)
	}
	clickRect(picked.rect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)

	if got := sess.Snapshot().Sort; got != ui.SortName {
		t.Errorf("sort after pick = %v, want %v (the pick must call setSort)", got, ui.SortName)
	}
	if len(m.sortMenuItems) != 0 {
		t.Errorf("dropdown still open after the pick (items %d, want 0)", len(m.sortMenuItems))
	}
	if !IdHasFocus(m.sortTriggerID) {
		t.Error("trigger lost keyboard focus after the menu pick")
	}
	t.Log("pick set SortName, closed the popup, and kept focus on the trigger")
}

// TestSortDropdown_EscCloses: with the detail panel open AND the sort
// dropdown open, the first Esc closes the DROPDOWN (consumed at render
// time, so handleGlobalKeys never sees it) and the panel stays open; the
// second Esc closes the panel.
func TestSortDropdown_EscCloses(t *testing.T) {
	sess, m, row := sortPopulated(t)
	// Select is synchronous and emits no event, so drain directly instead of
	// waiting on the event stream (the session has gone quiet by now).
	sess.Select(row.InstallDir)
	m.drain()
	if m.state.Selected != row.InstallDir {
		t.Fatalf("detail panel did not open for %q", row.InstallDir)
	}

	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	openSortDropdown(t, m, m.rootView)

	// First Esc: the open dropdown consumes it at render time.
	keyFrame(KeyEscape, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if len(m.sortMenuItems) != 0 {
		t.Errorf("dropdown still open after the first Esc (items %d, want 0)", len(m.sortMenuItems))
	}
	if got := sess.Snapshot().Selected; got != row.InstallDir {
		t.Errorf("first Esc closed the detail panel (selected %q): the dropdown must consume Esc before handleGlobalKeys", got)
	}
	if m.state.Selected != row.InstallDir {
		t.Errorf("model selection lost after the first Esc (selected %q): the panel must stay open", m.state.Selected)
	}

	// Second Esc: no dropdown open, so the global handler closes the panel.
	keyFrame(KeyEscape, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := sess.Snapshot().Selected; got != "" {
		t.Errorf("second Esc did not close the detail panel (selected %q)", got)
	}
	t.Log("Esc closed the dropdown first, the panel second")
}

// TestSortDropdown_ClickOutsideDismisses: a click outside both trigger and
// popup closes the dropdown without dispatching a sort change.
func TestSortDropdown_ClickOutsideDismisses(t *testing.T) {
	sess, m, _ := sortPopulated(t)
	openSortDropdown(t, m, m.rootView)

	// Empty grid space to the right of the single card: over neither the
	// trigger nor the popup.
	clickRect(Rect{Origin: Vec2{600, 400}, Size: Vec2{4, 4}}, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if len(m.sortMenuItems) != 0 {
		t.Errorf("dropdown still open after click-outside (items %d, want 0)", len(m.sortMenuItems))
	}
	if got := sess.Snapshot().Sort; got != ui.SortDefault {
		t.Errorf("click-outside changed the sort to %v, want unchanged (no dispatch)", got)
	}
	t.Log("click-outside closed the dropdown without dispatch")
}

// TestSortDropdown_DisabledWhenEmpty: with an empty library the trigger
// still renders (greyed out, mirroring the old Disabled attr) but neither
// click nor Enter/Space opens the dropdown.
func TestSortDropdown_DisabledWhenEmpty(t *testing.T) {
	sess, _ := guiFakes(t) // no scan: empty library
	m := newModel(Config{Session: sess})
	headlessFrames(t, 1100, 700)
	InputState.MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if !m.libraryEmpty() {
		t.Fatal("setup: library must be empty")
	}
	if m.sortTriggerID == nil {
		t.Fatal("disabled sort trigger not rendered (sortTriggerID nil)")
	}
	r := GetScreenRectOf(m.sortTriggerID)
	if r.Size[0] == 0 || r.Size[1] == 0 {
		t.Fatalf("disabled sort trigger rect unresolved: %+v", r)
	}

	clickRect(r, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if len(m.sortMenuItems) != 0 {
		t.Errorf("click on the disabled trigger opened the dropdown (items %d, want 0)", len(m.sortMenuItems))
	}

	focusSortTrigger(t, m, m.rootView)
	keyFrame(KeyEnter, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if len(m.sortMenuItems) != 0 {
		t.Errorf("Enter on the disabled trigger opened the dropdown (items %d, want 0)", len(m.sortMenuItems))
	}
	keyFrame(KeySpace, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if len(m.sortMenuItems) != 0 {
		t.Errorf("Space on the disabled trigger opened the dropdown (items %d, want 0)", len(m.sortMenuItems))
	}
	if got := sess.Snapshot().Sort; got != ui.SortDefault {
		t.Errorf("disabled trigger changed the sort to %v, want unchanged", got)
	}
	t.Log("empty library: sort trigger inert to click and keys")
}
