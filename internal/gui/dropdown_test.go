package gui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// dropdownFakes builds a session whose Versions(dir) is deterministic and
// offline: a cached v0.9.4-test bundle plus a pinned v0.8.0-test default,
// so an installed row's list is {installed, v0.9.4-test, v0.8.0-test}
// semver-desc without any network.
func dropdownFakes(t *testing.T) (*ui.Session, string) {
	t.Helper()
	return guiFakes(t, func(d *ui.Deps) {
		s := settings.Defaults()
		s.DefaultVersion = "v0.8.0-test"
		d.Settings = s
		dir := filepath.Join(d.CacheDir, "optiscaler", "v0.9.4-test")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Optiscaler_test.7z"), []byte("BUNDLE"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
}

// markExternal plants a synthetic OptiScaler dxgi.dll (PE version resource)
// so the scanned row comes back external with a known installed version.
func markExternal(t *testing.T, binDir string, quad [4]uint16) {
	t.Helper()
	marker := testutil.StringInfoPE(false, map[string]string{
		"ProductName":      "OptiScaler",
		"OriginalFilename": "OptiScaler.dll",
	}, quad)
	writeGUIFile(t, filepath.Join(binDir, "dxgi.dll"), string(marker))
}

// scanExternalRow scans the fake library and returns its single row,
// requiring an external install with a known version.
func scanExternalRow(t *testing.T, sess *ui.Session) ui.GameRow {
	t.Helper()
	row := scanOneRow(t, sess)
	if row.Status != domain.StatusExternal {
		t.Fatalf("row status %q, want %q (the marker must be detected)", row.Status, domain.StatusExternal)
	}
	if row.OptiScalerVersion == "" {
		t.Fatalf("external row has no OptiScalerVersion; the tick assertion cannot be proven")
	}
	return row
}

// openDropdown clicks the trigger of the row's version dropdown and settles
// one frame so the popup's row rects resolve.
func openDropdown(t *testing.T, m *model, dir string, view FrameFn) {
	t.Helper()
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	r := m.versionDDRects[dir]
	if r.Size[0] == 0 || r.Size[1] == 0 {
		t.Fatalf("version dropdown trigger not rendered for %q (rect %+v)", dir, r)
	}
	clickRect(r, view)
	keyFrame(KeyCodeNone, 0, view) // popup row rects resolve from the open frame
	if m.versionDDItemsFor != dir {
		t.Fatalf("dropdown for %q did not open (items owner %q)", dir, m.versionDDItemsFor)
	}
}

// TestVersionDropdown_InstalledRowOnly: an installed (external/committed)
// row renders the version-dropdown trigger in the pill row; a clean
// not-installed row renders no dropdown at all.
func TestVersionDropdown_InstalledRowOnly(t *testing.T) {
	t.Run("installed row renders the trigger", func(t *testing.T) {
		sess, gameRoot := dropdownFakes(t)
		markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 7, 0, 0})
		row := scanExternalRow(t, sess)
		m := newModel(Config{Session: sess})

		headlessFrames(t, 400, 800)
		InputState.MousePoint = Vec2{-50, -50}
		view := cardView(m, row)
		keyFrame(KeyCodeNone, 0, view)
		keyFrame(KeyCodeNone, 0, view)
		r := m.versionDDRects[row.InstallDir]
		if r.Size[0] == 0 || r.Size[1] == 0 {
			t.Errorf("installed row rendered no version dropdown trigger (rect %+v)", r)
		}
		t.Logf("installed row trigger rect: %+v", r)
	})

	t.Run("not-installed row renders no dropdown", func(t *testing.T) {
		sess, _ := dropdownFakes(t)
		row := scanOneRow(t, sess) // no marker: clean row
		if row.Status == domain.StatusExternal || row.Status == domain.StatusCommitted {
			t.Fatalf("row status %q, want a clean row", row.Status)
		}
		m := newModel(Config{Session: sess})

		headlessFrames(t, 400, 800)
		InputState.MousePoint = Vec2{-50, -50}
		view := cardView(m, row)
		keyFrame(KeyCodeNone, 0, view)
		keyFrame(KeyCodeNone, 0, view)
		if len(m.versionDDRects) != 0 {
			t.Errorf("clean row rendered version dropdown triggers: %+v", m.versionDDRects)
		}
		t.Log("clean row renders no dropdown")
	})
}

// TestVersionDropdown_OpenListsVersions: opening the trigger lists exactly
// Session.Versions(dir) with the current version ticked.
func TestVersionDropdown_OpenListsVersions(t *testing.T) {
	sess, gameRoot := dropdownFakes(t)
	markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 7, 0, 0})
	row := scanExternalRow(t, sess)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 400, 800)
	InputState.MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	openDropdown(t, m, row.InstallDir, view)

	want := sess.Versions(row.InstallDir)
	if len(m.versionDDItems) != len(want) {
		t.Fatalf("dropdown rows %d, want %d (%v)", len(m.versionDDItems), len(want), want)
	}
	ticked := 0
	for i, it := range m.versionDDItems {
		if it.version != want[i] {
			t.Errorf("row %d version %q, want %q (Session.Versions order)", i, it.version, want[i])
		}
		if it.ticked {
			ticked++
			if it.version != row.OptiScalerVersion {
				t.Errorf("ticked row %q, want the current %q", it.version, row.OptiScalerVersion)
			}
		}
	}
	if ticked != 1 {
		t.Errorf("ticked rows %d, want exactly 1 (the current version)", ticked)
	}
	t.Logf("open dropdown: %v, current %q ticked", want, row.OptiScalerVersion)
}

// TestVersionDropdown_SelectOtherDispatchesSwitch: clicking a DIFFERENT
// version row dispatches SwitchVersion(dir, version) and closes.
func TestVersionDropdown_SelectOtherDispatchesSwitch(t *testing.T) {
	sess, gameRoot := dropdownFakes(t)
	markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 7, 0, 0})
	row := scanExternalRow(t, sess)
	m := newModel(Config{Session: sess})
	type call struct{ dir, version string }
	var calls []call
	m.switchVersionFn = func(dir, version string) { calls = append(calls, call{dir, version}) }

	headlessFrames(t, 400, 800)
	InputState.MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	openDropdown(t, m, row.InstallDir, view)

	// Pick the first non-current row.
	target := -1
	for i, it := range m.versionDDItems {
		if it.version != row.OptiScalerVersion {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("no non-current version offered: %+v", m.versionDDItems)
	}
	picked := m.versionDDItems[target]
	if picked.rect.Size[0] == 0 {
		t.Fatalf("dropdown row rect unresolved: %+v", picked)
	}
	clickRect(picked.rect, view)
	keyFrame(KeyCodeNone, 0, view)

	if len(calls) != 1 {
		t.Fatalf("SwitchVersion dispatches %d, want 1", len(calls))
	}
	if calls[0].dir != row.InstallDir || calls[0].version != picked.version {
		t.Errorf("SwitchVersion(%q, %q), want (%q, %q)", calls[0].dir, calls[0].version, row.InstallDir, picked.version)
	}
	if m.versionDDItemsFor != "" {
		t.Errorf("dropdown still open after selection (owner %q)", m.versionDDItemsFor)
	}
	t.Logf("switch dispatched to %q; dropdown closed", picked.version)
}

// TestVersionDropdown_ReselectCurrentNoOp (S13): clicking the CURRENT
// version row dispatches NOTHING — the widget does not even call
// SwitchVersion — and still closes.
func TestVersionDropdown_ReselectCurrentNoOp(t *testing.T) {
	sess, gameRoot := dropdownFakes(t)
	markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 7, 0, 0})
	row := scanExternalRow(t, sess)
	m := newModel(Config{Session: sess})
	var calls int
	m.switchVersionFn = func(_, _ string) { calls++ }

	headlessFrames(t, 400, 800)
	InputState.MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	openDropdown(t, m, row.InstallDir, view)

	target := -1
	for i, it := range m.versionDDItems {
		if it.ticked {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatalf("current version row not found: %+v", m.versionDDItems)
	}
	clickRect(m.versionDDItems[target].rect, view)
	keyFrame(KeyCodeNone, 0, view)

	if calls != 0 {
		t.Errorf("re-selecting the current version dispatched %d SwitchVersion calls, want 0 (S13)", calls)
	}
	if m.versionDDItemsFor != "" {
		t.Errorf("dropdown still open after re-selecting current (owner %q)", m.versionDDItemsFor)
	}
	t.Log("S13: current-version click closed without dispatching")
}

// TestVersionDropdown_Dismissal: Esc and click-outside both close the
// dropdown without dispatching anything.
func TestVersionDropdown_Dismissal(t *testing.T) {
	setup := func(t *testing.T) (*model, ui.GameRow, *int, FrameFn) {
		sess, gameRoot := dropdownFakes(t)
		markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 7, 0, 0})
		row := scanExternalRow(t, sess)
		m := newModel(Config{Session: sess})
		calls := new(int)
		m.switchVersionFn = func(_, _ string) { *calls++ }
		headlessFrames(t, 400, 800)
		InputState.MousePoint = Vec2{-50, -50}
		return m, row, calls, cardView(m, row)
	}

	t.Run("Esc closes without dispatch", func(t *testing.T) {
		m, row, calls, view := setup(t)
		openDropdown(t, m, row.InstallDir, view)
		keyFrame(KeyEscape, 0, view)
		keyFrame(KeyCodeNone, 0, view)
		if m.versionDDItemsFor != "" {
			t.Errorf("dropdown still open after Esc (owner %q)", m.versionDDItemsFor)
		}
		if *calls != 0 {
			t.Errorf("Esc dispatched %d SwitchVersion calls, want 0", *calls)
		}
		t.Log("Esc closed the dropdown without dispatch")
	})

	t.Run("click-outside closes without dispatch", func(t *testing.T) {
		m, row, calls, view := setup(t)
		openDropdown(t, m, row.InstallDir, view)
		// A point in the card's outer padding: inside the Viewport but over
		// neither the trigger nor the popup.
		clickRect(Rect{Origin: Vec2{2, 2}, Size: Vec2{4, 4}}, view)
		keyFrame(KeyCodeNone, 0, view)
		if m.versionDDItemsFor != "" {
			t.Errorf("dropdown still open after click-outside (owner %q)", m.versionDDItemsFor)
		}
		if *calls != 0 {
			t.Errorf("click-outside dispatched %d SwitchVersion calls, want 0", *calls)
		}
		t.Log("click-outside closed the dropdown without dispatch")
	})
}

// TestVersionDropdown_OneOpenAtATime: opening one card's dropdown closes any
// other card's — the popup list always belongs to the most recently opened
// trigger.
func TestVersionDropdown_OneOpenAtATime(t *testing.T) {
	sess, gameRoot := dropdownFakes(t)
	markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 7, 0, 0})
	rowA := scanExternalRow(t, sess)

	// A second, manually added game with a different external version.
	dirB := filepath.Join(t.TempDir(), "GameB")
	binB := filepath.Join(dirB, "bin")
	writeGUIFile(t, filepath.Join(binB, "gameb.exe"), "GAME")
	markExternal(t, binB, [4]uint16{0, 6, 0, 0})
	sess.AddDirectory(dirB)
	// The row appears before the PE-probe enrichment lands, so wait for the
	// version itself, not just the row.
	rowBReady := func() ui.GameRow {
		for _, r := range sess.VisibleRows() {
			if r.InstallDir == dirB && r.OptiScalerVersion != "" {
				return r
			}
		}
		return ui.GameRow{}
	}
	deadline := time.Now().Add(15 * time.Second)
	rowB := rowBReady()
	for rowB.InstallDir == "" && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
		rowB = rowBReady()
	}
	if rowB.InstallDir == "" || rowB.OptiScalerVersion == rowA.OptiScalerVersion {
		t.Fatalf("second external row unusable: %+v (first was %q)", rowB, rowA.OptiScalerVersion)
	}

	m := newModel(Config{Session: sess})
	headlessFrames(t, 800, 800)
	InputState.MousePoint = Vec2{-50, -50}
	view := func() {
		Container(Attrs(Viewport, Row), func() {
			m.fitCards(800)
			m.gameCard(rowA)
			m.gameCard(rowB)
		})
	}

	openDropdown(t, m, rowA.InstallDir, view)
	if m.versionDDItemsFor != rowA.InstallDir {
		t.Fatalf("A's dropdown not open (owner %q)", m.versionDDItemsFor)
	}

	// Opening B must close A: the popup list belongs to B alone.
	openDropdown(t, m, rowB.InstallDir, view)
	if m.versionDDItemsFor != rowB.InstallDir {
		t.Fatalf("opening B did not take over the popup (owner %q, want %q)", m.versionDDItemsFor, rowB.InstallDir)
	}
	ticked := 0
	for _, it := range m.versionDDItems {
		if it.ticked {
			ticked++
			if it.version != rowB.OptiScalerVersion {
				t.Errorf("ticked %q, want B's current %q (A's popup must be gone)", it.version, rowB.OptiScalerVersion)
			}
		}
	}
	if ticked != 1 {
		t.Errorf("ticked rows %d, want exactly 1 after switching dropdowns", ticked)
	}
	t.Log("opening B's dropdown closed A's (one open at a time)")
}

// focusDDTrigger builds two frames and Tabs until the card's version
// dropdown trigger holds keyboard focus (the trigger renders before the
// card's buttons, so it is early in the tab cycle; the loop tolerates
// sibling focusables without hard-coding a count).
func focusDDTrigger(t *testing.T, m *model, view FrameFn) {
	t.Helper()
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if m.ddTriggerID == nil {
		t.Fatal("version dropdown trigger not rendered (ddTriggerID nil)")
	}
	for tabs := 1; tabs <= 4; tabs++ {
		keyFrame(KeyTab, 0, view)      // CycleFocusOnTab moves focus
		keyFrame(KeyCodeNone, 0, view) // focus change applies
		if IdHasFocus(m.ddTriggerID) {
			t.Logf("dropdown trigger focused after %d Tab(s)", tabs)
			return
		}
	}
	t.Fatalf("version dropdown trigger never took focus via Tab (ddTriggerID %v)", m.ddTriggerID)
}

// TestVersionDropdown_TabFocusRing: the trigger is a Tab stop (Focusable +
// CycleFocusOnTab) and draws its focus-ring branch (focusBorder) exactly
// while it holds keyboard focus (seam: m.ddFocusRing, mirroring
// m.listFocusRing).
func TestVersionDropdown_TabFocusRing(t *testing.T) {
	sess, gameRoot := dropdownFakes(t)
	markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 7, 0, 0})
	row := scanExternalRow(t, sess)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 400, 800)
	InputState.MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if m.ddFocusRing {
		t.Error("focus ring drawn while the trigger is not focused")
	}

	focusDDTrigger(t, m, view)
	if !m.ddFocusRing {
		t.Error("focus ring not drawn while the trigger holds keyboard focus")
	}
	t.Log("Tab focused the trigger and the focus ring drew only while focused")
}

// TestVersionDropdown_EnterSpaceToggle: Enter and Space on the FOCUSED
// trigger each toggle the dropdown open and closed, dispatching nothing.
func TestVersionDropdown_EnterSpaceToggle(t *testing.T) {
	sess, gameRoot := dropdownFakes(t)
	markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 7, 0, 0})
	row := scanExternalRow(t, sess)
	m := newModel(Config{Session: sess})
	var calls int
	m.switchVersionFn = func(_, _ string) { calls++ }

	headlessFrames(t, 400, 800)
	InputState.MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	focusDDTrigger(t, m, view)

	toggle := func(key KeyCode) {
		keyFrame(key, 0, view)
		keyFrame(KeyCodeNone, 0, view) // popup open/close settles
	}
	for _, key := range []KeyCode{KeyEnter, KeySpace} {
		toggle(key)
		if m.versionDDItemsFor != row.InstallDir {
			t.Errorf("%v on the focused trigger did not open the dropdown (owner %q)", key, m.versionDDItemsFor)
		}
		toggle(key)
		if m.versionDDItemsFor != "" {
			t.Errorf("second %v on the focused trigger did not close the dropdown (owner %q)", key, m.versionDDItemsFor)
		}
	}
	if calls != 0 {
		t.Errorf("keyboard toggling dispatched %d SwitchVersion calls, want 0", calls)
	}
	if !IdHasFocus(m.ddTriggerID) {
		t.Error("trigger lost keyboard focus across the open/close toggles")
	}
	t.Log("Enter and Space each toggled the dropdown open and closed")
}

// TestVersionDropdown_EscClosesDropdownBeforePanel: with the detail panel
// open AND its version dropdown open, the first Esc closes the DROPDOWN
// (consumed at render time, so handleGlobalKeys never sees it) and the
// panel stays open; the second Esc closes the panel.
func TestVersionDropdown_EscClosesDropdownBeforePanel(t *testing.T) {
	sess, gameRoot := dropdownFakes(t)
	markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 7, 0, 0})
	row := scanExternalRow(t, sess)
	m := newModel(Config{Session: sess})
	var calls int
	m.switchVersionFn = func(_, _ string) { calls++ }
	sess.Select(row.InstallDir)
	deadline := time.Now().Add(15 * time.Second)
	for m.state.Selected != row.InstallDir && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
			m.drain()
		case <-time.After(20 * time.Millisecond):
		}
	}
	if m.state.Selected != row.InstallDir {
		t.Fatalf("detail panel never opened for %q", row.InstallDir)
	}

	headlessFrames(t, 1100, 1400)
	InputState.MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	openDropdown(t, m, row.InstallDir, m.rootView)

	// First Esc: the open dropdown consumes it at render time.
	keyFrame(KeyEscape, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.versionDDItemsFor != "" {
		t.Errorf("dropdown still open after the first Esc (owner %q)", m.versionDDItemsFor)
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
	if calls != 0 {
		t.Errorf("Esc dismissal dispatched %d SwitchVersion calls, want 0", calls)
	}
	t.Log("Esc closed the dropdown first, the panel second")
}

// TestVersionDropdown_DetailPanelWired: the detail panel's pill row also
// renders the dropdown trigger for an installed row.
func TestVersionDropdown_DetailPanelWired(t *testing.T) {
	sess, gameRoot := dropdownFakes(t)
	markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 7, 0, 0})
	row := scanExternalRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)
	deadline := time.Now().Add(15 * time.Second)
	for m.state.Selected != row.InstallDir && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
			m.drain()
		case <-time.After(20 * time.Millisecond):
		}
	}

	headlessFrames(t, 1100, 1400)
	InputState.MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if r := m.versionDDRects[row.InstallDir]; r.Size[0] == 0 || r.Size[1] == 0 {
		t.Errorf("detail panel rendered no version dropdown trigger for the selected installed row")
	}
	t.Log("detail panel renders the version dropdown")
}
