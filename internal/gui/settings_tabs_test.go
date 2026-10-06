package gui

import (
	"testing"

	. "go.hasen.dev/shirei"
)

// TestGUISettingsTabDefaultsToGeneral: opening the settings modal always
// lands on the General tab, even if a previous visit left the Optiscaler
// tab active.
func TestGUISettingsTabDefaultsToGeneral(t *testing.T) {
	sess, _ := guiFakesWithDirs(t)
	m := newModel(Config{Session: sess})

	m.settingsTab = settingsTabOptiscaler // stale state from a previous open
	m.openSettings()
	if m.settingsTab != settingsTabGeneral {
		t.Errorf("settingsTab = %v after openSettings, want General", m.settingsTab)
	}
}

// TestGUISettingsTabSwitchViaKeyboard: the trap auto-focuses the General
// tab on open; Tab focuses the Optiscaler tab and Enter switches to it,
// after which the next Tab reaches the version field (the Optiscaler
// tab's first content stop).
func TestGUISettingsTabSwitchViaKeyboard(t *testing.T) {
	sess, _ := guiFakesWithDirs(t)
	m := newModel(Config{Session: sess})
	m.openSettings()

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // open frame: trap auto-focuses the General tab
	keyFrame(KeyTab, 0, m.rootView)      // Optiscaler tab
	keyFrame(KeyEnter, 0, m.rootView)    // switch
	keyFrame(KeyCodeNone, 0, m.rootView) // settle

	if m.settingsTab != settingsTabOptiscaler {
		t.Fatalf("settingsTab = %v after Tab+Enter on the Optiscaler tab, want Optiscaler", m.settingsTab)
	}

	// One Tab from the Optiscaler tab button reaches the version field;
	// typing lands in the version buffer.
	version0 := m.versionBuf
	keyFrame(KeyTab, 0, m.rootView)
	GetFrameInput().Text = "x"
	keyFrame(KeyCodeNone, 0, m.rootView)
	GetFrameInput().Text = ""
	if m.versionBuf != version0+"x" {
		t.Errorf("version buffer %q after typing on the Optiscaler tab, want %q", m.versionBuf, version0+"x")
	}
}

// TestGUISettingsTabArrowKeys: Left/Right on a focused tab switch tabs.
func TestGUISettingsTabArrowKeys(t *testing.T) {
	sess, _ := guiFakesWithDirs(t)
	m := newModel(Config{Session: sess})
	m.openSettings()

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // General tab focused
	keyFrame(KeyRight, 0, m.rootView)    // switch right
	keyFrame(KeyCodeNone, 0, m.rootView) // settle
	if m.settingsTab != settingsTabOptiscaler {
		t.Fatalf("settingsTab = %v after Right on a focused tab, want Optiscaler", m.settingsTab)
	}
	keyFrame(KeyLeft, 0, m.rootView) // switch back
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.settingsTab != settingsTabGeneral {
		t.Errorf("settingsTab = %v after Left on a focused tab, want General", m.settingsTab)
	}
}

// TestGUISettingsGeneralTabHidesOptiscalerFields: on the General tab the
// focus cycle goes straight from the tab bar to the online-lookups
// toggle — the version field lives on the Optiscaler tab only.
func TestGUISettingsGeneralTabHidesOptiscalerFields(t *testing.T) {
	sess, _ := guiFakesWithDirs(t)
	m := newModel(Config{Session: sess})
	m.openSettings()

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // open frame: General tab focused
	keyFrame(KeyTab, 0, m.rootView)      // Optiscaler tab
	keyFrame(KeyTab, 0, m.rootView)      // online-lookups toggle (no version field in between)
	keyFrame(KeyEnter, 0, m.rootView)    // arm the flip
	keyFrame(KeyCodeNone, 0, m.rootView) // release flips (press->release)

	if sess.Settings().OnlineLookups {
		t.Error("OnlineLookups still true after 2 Tabs + Enter on the General tab; the version field must not sit between the tab bar and the toggle")
	}
}

// TestGUISettingsModalKeepsHeightAcrossTabs: the modal must not resize
// when switching tabs — the content area holds the tallest height seen
// while the modal is open.
func TestGUISettingsModalKeepsHeightAcrossTabs(t *testing.T) {
	sess, _ := guiFakesWithDirs(t, "/games/alpha", "/games/beta")
	m := newModel(Config{Session: sess})
	m.openSettings()

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // settle: measurement converges
	hGeneral := m.settingsContentRect.Size[1]
	if hGeneral == 0 {
		t.Fatal("settings content height not measured on the General tab")
	}

	keyFrame(KeyTab, 0, m.rootView)      // Optiscaler tab
	keyFrame(KeyEnter, 0, m.rootView)    // switch
	keyFrame(KeyCodeNone, 0, m.rootView) // settle
	keyFrame(KeyCodeNone, 0, m.rootView)
	if got := m.settingsContentRect.Size[1]; got != hGeneral {
		t.Errorf("settings content height = %v on the Optiscaler tab, want %v (modal must keep its height across tabs)", got, hGeneral)
	}
}

// TestGUISettingsTabSwitchViaMouse: clicking the Optiscaler tab switches
// the modal to it AND moves keyboard focus to it (the focus ring is the
// selection read-out; a click that leaves the ring on General looks like
// "General stays selected").
func TestGUISettingsTabSwitchViaMouse(t *testing.T) {
	sess, _ := guiFakesWithDirs(t)
	m := newModel(Config{Session: sess})
	m.openSettings()

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build + register tab rects/ids
	r := m.settingsTabRects[settingsTabOptiscaler]
	if r.Size[0] == 0 {
		t.Fatal("Optiscaler tab rect not registered")
	}
	clickRect(r, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView) // settle

	if m.settingsTab != settingsTabOptiscaler {
		t.Fatalf("settingsTab = %v after clicking the Optiscaler tab, want Optiscaler", m.settingsTab)
	}
	if id := m.settingsTabIDs[settingsTabOptiscaler]; id == nil || !IdHasFocus(id) {
		t.Error("clicking the Optiscaler tab did not move keyboard focus to it (ring still on General)")
	}
	if id := m.settingsTabIDs[settingsTabGeneral]; id != nil && IdHasFocus(id) {
		t.Error("General tab kept keyboard focus after the Optiscaler tab was clicked")
	}
}
