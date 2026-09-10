package gui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// dlssGUIFakes wires guiFakes with a DLSS client against a fake NVIDIA
// endpoint and plants the three versioned NVIDIA DLLs in the game's bin.
// gate, when non-nil, blocks the fake commit endpoint so an update op can
// be held in-flight deterministically.
func dlssGUIFakes(t *testing.T, gate chan struct{}) (*ui.Session, string, string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/NVIDIA/DLSS/commits/main", func(w http.ResponseWriter, r *http.Request) {
		if gate != nil {
			<-gate
		}
		_, _ = w.Write([]byte(`{"sha":"` + strings.Repeat("d", 40) + `"}`))
	})
	mux.HandleFunc("/NVIDIA/DLSS/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(testutil.FixedVersionPE(310, 5, 3, 0))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	var cacheRoot string
	sess, gameRoot := guiFakes(t, func(d *ui.Deps) {
		d.DLSS = dlss.NewWithBaseURLs(srv.Client(), srv.URL, srv.URL)
		cacheRoot = d.CacheDir
	})
	bin := filepath.Join(gameRoot, "bin")
	for i, name := range dlss.Files {
		writeGUIFile(t, filepath.Join(bin, name), string(testutil.FixedVersionPE(3, 7, uint16(20+i), 0)))
	}
	return sess, gameRoot, cacheRoot
}

// waitSessEvent drains session events until kind arrives.
func waitSessEvent(t *testing.T, sess *ui.Session, kind ui.EventKind) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case ev := <-sess.Events():
			if ev.Kind == kind {
				return
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatalf("timed out waiting for event %v", kind)
}

func dlssVersionOnDisk(t *testing.T, gameRoot, name string) string {
	t.Helper()
	v, err := pever.FileVersion(filepath.Join(gameRoot, "bin", name))
	if err != nil {
		t.Fatalf("%s unreadable: %v", name, err)
	}
	return v
}

// TestDLSSControl_RendersOnCardsAndPanel: a row with a DLSS component
// renders the dual-color control (update area + restore arrow) on the card
// AND in the detail panel; a row without DLSS renders neither.
func TestDLSSControl_RendersOnCardsAndPanel(t *testing.T) {
	sess, _, _ := dlssGUIFakes(t, nil)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 400, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if m.dlssUpdateRect.Size[0] == 0 || m.dlssArrowRect.Size[0] == 0 {
		t.Fatalf("DLSS control not rendered on card: update %+v arrow %+v", m.dlssUpdateRect, m.dlssArrowRect)
	}

	// Detail panel renders the same control (the row must be selected —
	// without a selection the panel renders nothing and stale card rects
	// would satisfy this assertion vacuously).
	sess.Select(row.InstallDir)
	m.state = sess.Snapshot()
	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame
	if m.dlssUpdateRect.Size[0] == 0 || m.dlssArrowRect.Size[0] == 0 {
		t.Fatalf("DLSS control not rendered in detail panel: update %+v arrow %+v", m.dlssUpdateRect, m.dlssArrowRect)
	}
	t.Logf("card control rects: update %+v arrow %+v", m.dlssUpdateRect, m.dlssArrowRect)

	// A game without the NVIDIA set renders no control: use a model whose
	// row lacks DLSS components (plain guiFakes row has unparseable dll).
	plainSess, plainRoot := guiFakes(t)
	plain := scanOneRow(t, plainSess)
	pm := newModel(Config{Session: plainSess})
	_ = plainRoot
	plainView := cardView(pm, plain)
	GetInputState().MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, plainView)
	keyFrame(KeyCodeNone, 0, plainView)
	if pm.dlssUpdateRect.Size[0] != 0 || pm.dlssArrowRect.Size[0] != 0 {
		t.Errorf("plain game rendered a DLSS control: %+v / %+v", pm.dlssUpdateRect, pm.dlssArrowRect)
	}
}

// externalDLSSFakes makes the fake game an external (hand-installed)
// OptiScaler row — branded dxgi.dll, no manifest — carrying the three
// versioned NVIDIA DLLs, so the scan's external probe and the DLSS pill
// share one library.
func externalDLSSFakes(t *testing.T) (*ui.Session, string) {
	t.Helper()
	sess, gameRoot, _ := dlssGUIFakes(t, nil)
	writeGUIFile(t, filepath.Join(gameRoot, "bin", "dxgi.dll"),
		string(testutil.StringInfoPE(false, map[string]string{
			"ProductName":      "OptiScaler",
			"OriginalFilename": "OptiScaler.dll",
		}, [4]uint16{0, 9, 4, 0})))
	return sess, gameRoot
}

// TestDLSSControl_ExternalRow: a hand-installed (external) OptiScaler game
// carrying the NVIDIA runtime set renders the DLSS update/restore control
// in the detail panel, whose status/version pills sit UNDER the poster (the
// long-standing layout). At a small window they fit in the fold; at a wide
// window the 2:3 cover pushes them past it and the panel's wheel scroll
// brings them back — reachability without reordering the pane.
func TestDLSSControl_ExternalRow(t *testing.T) {
	sess, _ := externalDLSSFakes(t)
	row := scanOneRow(t, sess)
	if row.Status != domain.StatusExternal {
		t.Fatalf("setup: status %q, want external (branded dxgi.dll undetected)", row.Status)
	}
	hasDLSSBadge := false
	for _, b := range row.TechBadges {
		if b.Label == "DLSS" {
			hasDLSSBadge = true
		}
	}
	if !hasDLSSBadge {
		t.Fatalf("setup: card carries no DLSS tech badge: %+v", row.TechBadges)
	}
	sess.Select(row.InstallDir)

	// Small window: the pill row fits below the poster.
	m := newModel(Config{Session: sess})
	m.state = sess.Snapshot()
	headlessFrames(t, 1100, 700)
	GetInputState().MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame
	if m.dlssUpdateRect.Size[0] == 0 || m.dlssArrowRect.Size[0] == 0 {
		t.Fatalf("small window: external row rendered no DLSS control in the detail panel: update %+v arrow %+v", m.dlssUpdateRect, m.dlssArrowRect)
	}

	// Wide window: the pills sit under the poster, past the fold. Wheel
	// input over the panel scrolls them into view.
	m = newModel(Config{Session: sess})
	m.state = sess.Snapshot()
	headlessFrames(t, 1600, 900)
	keyFrame(KeyCodeNone, 0, m.rootView) // build (captures detailPanelRect)
	panel := m.detailPanelRect
	if panel.Size[0] == 0 {
		t.Fatal("setup: detail panel rect empty")
	}
	GetInputState().MousePoint = Vec2{panel.Origin[0] + panel.Size[0]/2, panel.Origin[1] + panel.Size[1]/2}
	for i := 0; i < 3; i++ {
		keyFrame(KeyCodeNone, 0, m.rootView) // hover settles
		GetFrameInput().Scroll = Vec2{0, 600}
		RunFrameFn(m.rootView)
		GetFrameInput().Scroll = Vec2{}
	}
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame
	if m.dlssUpdateRect.Size[0] == 0 || m.dlssArrowRect.Size[0] == 0 {
		t.Fatalf("wide window after wheel scroll: no DLSS control: update %+v arrow %+v", m.dlssUpdateRect, m.dlssArrowRect)
	}
}

// strippedDLSSFakes replaces the planted NVIDIA DLLs with version-stripped
// PE images — the shape of real installs whose version resources do not
// parse, the exact card-badge-but-no-detail-pill report.
func strippedDLSSFakes(t *testing.T) (*ui.Session, string, string) {
	t.Helper()
	sess, gameRoot, cacheRoot := dlssGUIFakes(t, nil)
	bin := filepath.Join(gameRoot, "bin")
	for _, name := range dlss.Files {
		writeGUIFile(t, filepath.Join(bin, name), string(testutil.StringInfoPE(false, nil, [4]uint16{})))
	}
	return sess, gameRoot, cacheRoot
}

// TestDLSSControl_StrippedVersionStillUpdatable: a game whose NVIDIA DLLs
// have no readable version still renders the DLSS pill (bare "DLSS", same
// label the card's tech badge shows) and the pill remains the interactive
// update control — pressing it installs the published set. The pill row
// sits under the poster; the panel's wheel scroll brings it into view when
// the fold cuts it (long install paths wrap and push it down).
func TestDLSSControl_StrippedVersionStillUpdatable(t *testing.T) {
	sess, gameRoot, _ := strippedDLSSFakes(t)
	row := scanOneRow(t, sess)
	if !row.DLSSReady {
		t.Fatalf("setup: complete set not marked ready (components %v)", row.Components)
	}
	if row.DLSSVersion != "" {
		t.Fatalf("setup: stripped DLL parsed a version %q", row.DLSSVersion)
	}
	var dlssPill string
	for _, c := range row.Components {
		if c == "DLSS" {
			dlssPill = c
			break
		}
	}
	if dlssPill != "DLSS" {
		t.Fatalf("stripped DLSS pill missing: components %v", row.Components)
	}

	sess.Select(row.InstallDir)
	m := newModel(Config{Session: sess})
	m.state = sess.Snapshot()
	headlessFrames(t, 1100, 700)
	GetInputState().MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	panel := m.detailPanelRect
	if panel.Size[0] == 0 {
		t.Fatal("setup: detail panel rect empty")
	}
	GetInputState().MousePoint = Vec2{panel.Origin[0] + panel.Size[0]/2, panel.Origin[1] + panel.Size[1]/2}
	for i := 0; i < 3; i++ {
		keyFrame(KeyCodeNone, 0, m.rootView) // hover settles
		GetFrameInput().Scroll = Vec2{0, 600}
		RunFrameFn(m.rootView)
		GetFrameInput().Scroll = Vec2{}
	}
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame
	ur := m.dlssUpdateRect
	if ur.Size[0] == 0 {
		t.Fatalf("version-stripped game rendered no DLSS control after scroll (pill %q)", dlssPill)
	}
	clickRect(ur, m.rootView)
	waitSessEvent(t, sess, ui.EvOpDone)
	for _, name := range dlss.Files {
		if got := dlssVersionOnDisk(t, gameRoot, name); got != "310.5.3.0" {
			t.Errorf("%s after click = %q, want 310.5.3.0", name, got)
		}
	}
}

// TestDLSSControl_CachedMarkerOffline: with online lookups off and a newer
// set in the download cache, the pill's version segment renders the cached
// version as the update marker — the cached half of the availability check
// must be visible, not just the TUI line. Measured as the version segment
// growing once the check fills DLSSCached (the bare pill's version area is
// near-empty otherwise).
func TestDLSSControl_CachedMarkerOffline(t *testing.T) {
	sess, _, cacheRoot := strippedDLSSFakes(t)
	row := scanOneRow(t, sess)
	sess.Select(row.InstallDir)
	m := newModel(Config{Session: sess})
	m.state = sess.Snapshot()

	revealWidth := func() float32 {
		headlessFrames(t, 1100, 700)
		GetInputState().MousePoint = Vec2{-50, -50}
		keyFrame(KeyCodeNone, 0, m.rootView) // build
		panel := m.detailPanelRect
		if panel.Size[0] == 0 {
			t.Fatal("setup: detail panel rect empty")
		}
		GetInputState().MousePoint = Vec2{panel.Origin[0] + panel.Size[0]/2, panel.Origin[1] + panel.Size[1]/2}
		for i := 0; i < 3; i++ {
			keyFrame(KeyCodeNone, 0, m.rootView)
			GetFrameInput().Scroll = Vec2{0, 600}
			RunFrameFn(m.rootView)
			GetFrameInput().Scroll = Vec2{}
		}
		keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame
		return m.dlssUpdateRect.Size[0]
	}
	base := revealWidth()
	if base == 0 {
		t.Fatal("no DLSS control before the cached marker")
	}

	commit := strings.Repeat("a", 40)
	files := map[string][]byte{}
	for i, name := range dlss.Files {
		files[name] = testutil.FixedVersionPE(310, 9, uint16(i), 0)
	}
	testutil.SeedDLSSCacheDir(t, cacheRoot, commit, files)
	sess.CheckDLSS(context.Background()) // online lookups off: fills the cached half only
	m.state = sess.Snapshot()
	if m.state.DLSSCached != "310.9.0.0" {
		t.Fatalf("setup: DLSSCached %q, want 310.9.0.0", m.state.DLSSCached)
	}
	marked := revealWidth()
	if marked <= base {
		t.Errorf("version segment did not grow with the cached marker: base %.1f, marked %.1f", base, marked)
	}
}

// TestDLSSUpdateTargetMarker: the update-available marker appended to the
// DLSS control's version segment appears exactly when a known candidate —
// the startup check's published version, or the download cache when the
// online half is unknown (offline mode, failed lookup) — is newer than the
// applied version, or the applied version is unreadable.
func TestDLSSUpdateTargetMarker(t *testing.T) {
	cases := []struct {
		name    string
		applied string
		online  string
		cached  string
		want    string
	}{
		{"nothing known", "3.7.20.0", "", "", ""},
		{"older applied, online known", "3.7.20.0", "310.9.1", "", "310.9.1"},
		{"current applied", "310.9.1.0", "310.9.1", "", ""},
		{"newer applied", "310.10.0.0", "310.9.1", "", ""},
		{"unreadable applied", "", "310.9.1", "", "310.9.1"},
		{"online unknown, cached newer", "3.7.20.0", "", "310.5.3.0", "310.5.3.0"},
		{"online unknown, cached current", "310.5.3.0", "", "310.5.3.0", ""},
		{"online unknown, cached older", "310.9.0.0", "", "310.5.3.0", ""},
		{"online unknown, cached unreadable", "", "", "310.5.3.0", "310.5.3.0"},
		{"cached fresher than lagging tag", "3.7.20.0", "310.9.1", "310.9.2.0", "310.9.2.0"},
		{"online wins over older cache", "3.7.20.0", "310.9.1", "310.5.3.0", "310.9.1"},
	}
	for _, tc := range cases {
		if got := dlssUpdateTarget(&ui.GameRow{DLSSVersion: tc.applied}, tc.online, tc.cached); got != tc.want {
			t.Errorf("%s: dlssUpdateTarget = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestDLSSUpdateClick_FiresUpdateNotSelect: clicking the version area
// starts the update op and never selects the card.
func TestDLSSUpdateClick_FiresUpdateNotSelect(t *testing.T) {
	sess, gameRoot, _ := dlssGUIFakes(t, nil)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 400, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	ur := m.dlssUpdateRect
	if ur.Size[0] == 0 {
		t.Fatal("no DLSS update rect")
	}
	clickRect(ur, view)

	waitSessEvent(t, sess, ui.EvOpDone)
	if got := sess.Snapshot().Selected; got == row.InstallDir {
		t.Errorf("DLSS update click selected the card (%q)", got)
	}
	for _, name := range dlss.Files {
		if got := dlssVersionOnDisk(t, gameRoot, name); got != "310.5.3.0" {
			t.Errorf("%s after click = %q, want 310.5.3.0", name, got)
		}
	}
	if snaps := sess.DLSSSnapshots(row.InstallDir); len(snaps) != 1 {
		t.Errorf("snapshots after click %d, want 1", len(snaps))
	}
}

// TestDLSSRestoreMenu_FullFlow: the arrow opens the menu listing backed-up
// sets, Esc closes it, a menu pick asks for confirmation, and accepting
// restores the complete set.
func TestDLSSRestoreMenu_FullFlow(t *testing.T) {
	sess, gameRoot, _ := dlssGUIFakes(t, nil)
	row := scanOneRow(t, sess)

	// Produce one backup via a real update.
	sess.UpdateDLSS(row.InstallDir)
	waitSessEvent(t, sess, ui.EvOpDone)

	m := newModel(Config{Session: sess})
	headlessFrames(t, 400, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	row = sess.VisibleRows()[0] // Components refreshed to DLSS 4.5
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	ar := m.dlssArrowRect
	if ar.Size[0] == 0 {
		t.Fatal("no DLSS arrow rect")
	}
	clickRect(ar, view)
	keyFrame(KeyCodeNone, 0, view) // popup rows render
	if m.openDLSSDir != row.InstallDir {
		t.Fatalf("menu did not open for %q (open %q)", row.InstallDir, m.openDLSSDir)
	}
	if len(m.dlssSnapshotItems) != 1 {
		t.Fatalf("menu rows %d, want 1", len(m.dlssSnapshotItems))
	}
	if item := m.dlssSnapshotItems[0]; item.id == "" || item.label == "" {
		t.Errorf("menu item incomplete: %+v", item)
	} else {
		t.Logf("menu item %q (id %s)", item.label, item.id[:8])
	}

	// Esc closes without dispatching.
	keyFrame(KeyEscape, 0, view)
	if m.openDLSSDir != "" {
		t.Fatal("Esc did not close the restore menu")
	}

	// Reopen and pick the entry: confirmation appears, accept restores.
	clickRect(ar, view)
	keyFrame(KeyCodeNone, 0, view)
	clickRect(m.dlssSnapshotItems[0].rect, view)
	deadline := time.Now().Add(15 * time.Second)
	for sess.Snapshot().Confirm == nil && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	c := sess.Snapshot().Confirm
	if c == nil || c.Kind != ui.ConfirmDLSSRestore {
		t.Fatalf("menu pick produced %+v, want ConfirmDLSSRestore", c)
	}
	sess.AnswerConfirm(true)
	waitSessEvent(t, sess, ui.EvOpDone)
	for i, name := range dlss.Files {
		want := fmt.Sprintf("3.7.%d.0", 20+i)
		if got := dlssVersionOnDisk(t, gameRoot, name); got != want {
			t.Errorf("%s after restore = %q, want %q", name, got, want)
		}
	}
}

// TestDLSSControl_BusyGameShowsStaticPill: while a per-game op runs the
// control collapses to the static pill (no click targets, no menu).
func TestDLSSControl_BusyGameShowsStaticPill(t *testing.T) {
	gate := make(chan struct{})
	sess, gameRoot, _ := dlssGUIFakes(t, gate)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.UpdateDLSS(row.InstallDir) // held in-flight by the gated endpoint
	deadline := time.Now().Add(15 * time.Second)
	for sess.Snapshot().Busy == "" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sess.Snapshot().Busy == "" {
		t.Fatal("op never became busy; the gate did not hold it")
	}
	headlessFrames(t, 400, 800)
	view := func() {
		Container(Attrs(Viewport), func() {
			m.fitCards(400)
			busy := m.sess.VisibleRows()[0]
			m.gameCard(busy, 0)
		})
	}
	keyFrame(KeyCodeNone, 0, view)
	close(gate) // release the op; it settles after the assertions below
	if m.dlssUpdateRect.Size[0] != 0 || m.dlssArrowRect.Size[0] != 0 {
		t.Errorf("busy game rendered DLSS control rects: %+v / %+v", m.dlssUpdateRect, m.dlssArrowRect)
	}
	if m.openDLSSDir != "" {
		t.Errorf("busy game kept a restore menu open for %q", m.openDLSSDir)
	}
	waitSessEvent(t, sess, ui.EvOpDone)
	if got := dlssVersionOnDisk(t, gameRoot, dlss.Files[0]); got != "310.5.3.0" {
		t.Errorf("released update did not complete: %q", got)
	}
}

// TestDLSSMenuRowsKeyboardPickable: Tab from the arrow walks into the menu
// rows and Enter on a focused row dispatches the restore pick — the menu
// is keyboard-reachable, not mouse-only.
func TestDLSSMenuRowsKeyboardPickable(t *testing.T) {
	sess, _, _ := dlssGUIFakes(t, nil)
	row := scanOneRow(t, sess)
	sess.UpdateDLSS(row.InstallDir)
	waitSessEvent(t, sess, ui.EvOpDone)

	m := newModel(Config{Session: sess})
	var restored []string
	m.dlssRestoreFn = func(_, id string) { restored = append(restored, id) }

	headlessFrames(t, 400, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	row = sess.VisibleRows()[0]
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	ar := m.dlssArrowRect
	if ar.Size[0] == 0 {
		t.Fatal("no DLSS arrow rect")
	}
	clickRect(ar, view)
	keyFrame(KeyCodeNone, 0, view) // menu rows render
	if len(m.dlssSnapshotItems) != 1 {
		t.Fatalf("menu rows %d, want 1", len(m.dlssSnapshotItems))
	}
	cid := m.dlssSnapshotItems[0].cid

	focused := false
	for tabs := 1; tabs <= 6; tabs++ {
		keyFrame(KeyTab, 0, view)
		keyFrame(KeyCodeNone, 0, view) // focus change settles
		if IdHasFocus(cid) {
			focused = true
			t.Logf("menu row focused after %d Tab(s)", tabs)
			break
		}
	}
	if !focused {
		t.Fatal("menu row never took keyboard focus via Tab")
	}

	keyFrame(KeyEnter, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if len(restored) != 1 {
		t.Fatalf("Enter on the focused row dispatched %d restores, want 1", len(restored))
	}
	if m.openDLSSDir != "" {
		t.Errorf("pick did not close the menu (open %q)", m.openDLSSDir)
	}
}

// TestDLSSMenuRendersCapturedSnapshotList: the menu renders the list
// captured at open (m.dlssSnaps) — the backup directory is read once at
// open, not per frame, and the rows stay stable across settle frames.
func TestDLSSMenuRendersCapturedSnapshotList(t *testing.T) {
	sess, _, _ := dlssGUIFakes(t, nil)
	row := scanOneRow(t, sess)
	sess.UpdateDLSS(row.InstallDir)
	waitSessEvent(t, sess, ui.EvOpDone)

	m := newModel(Config{Session: sess})
	headlessFrames(t, 400, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	row = sess.VisibleRows()[0]
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	clickRect(m.dlssArrowRect, view)
	keyFrame(KeyCodeNone, 0, view)
	if len(m.dlssSnaps) != 1 || len(m.dlssSnapshotItems) != 1 {
		t.Fatalf("open menu state: captured %d snapshots, %d rows rendered", len(m.dlssSnaps), len(m.dlssSnapshotItems))
	}
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if len(m.dlssSnapshotItems) != 1 {
		t.Errorf("open menu did not keep its captured rows across settle frames")
	}
}
