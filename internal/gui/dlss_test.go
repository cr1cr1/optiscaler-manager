package gui

import (
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
func dlssGUIFakes(t *testing.T, gate chan struct{}) (*ui.Session, string) {
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
	sess, gameRoot := guiFakes(t, func(d *ui.Deps) {
		d.DLSS = dlss.NewWithBaseURLs(srv.Client(), srv.URL, srv.URL)
	})
	bin := filepath.Join(gameRoot, "bin")
	for i, name := range dlss.Files {
		writeGUIFile(t, filepath.Join(bin, name), string(testutil.FixedVersionPE(3, 7, uint16(20+i), 0)))
	}
	return sess, gameRoot
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
	sess, _ := dlssGUIFakes(t, nil)
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
	sess, gameRoot := dlssGUIFakes(t, nil)
	writeGUIFile(t, filepath.Join(gameRoot, "bin", "dxgi.dll"),
		string(testutil.StringInfoPE(false, map[string]string{
			"ProductName":      "OptiScaler",
			"OriginalFilename": "OptiScaler.dll",
		}, [4]uint16{0, 9, 4, 0})))
	return sess, gameRoot
}

// TestDLSSControl_ExternalRow: a hand-installed (external) OptiScaler game
// carrying the NVIDIA runtime set renders the DLSS update/restore control
// in the detail panel at BOTH a small and a wide window. The card's static
// DLSS tech badge must not be the only affordance: the control lives on
// the component pill, which external rows suppressed entirely (the pills
// were also folded out of view behind the 2:3 cover at wide panels).
// Card-pill clipping at the fixed card width is a pre-existing card layout
// property; the panel is the actionable surface.
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

	for _, w := range []struct {
		name string
		w, h int
	}{
		{"small", 1100, 700}, // 330px panel: the 2:3 cover clipped the pill row
		{"wide", 1600, 900},  // 480px panel: the cover alone overflowed the fold
	} {
		m := newModel(Config{Session: sess})
		m.state = sess.Snapshot()
		headlessFrames(t, w.w, w.h)
		GetInputState().MousePoint = Vec2{-50, -50}
		keyFrame(KeyCodeNone, 0, m.rootView) // build
		keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame
		if m.dlssUpdateRect.Size[0] == 0 || m.dlssArrowRect.Size[0] == 0 {
			t.Errorf("%s window: external row rendered no DLSS control in the detail panel: update %+v arrow %+v", w.name, m.dlssUpdateRect, m.dlssArrowRect)
		}
	}
}

// TestDLSSUpdateClick_FiresUpdateNotSelect: clicking the version area
// starts the update op and never selects the card.
func TestDLSSUpdateClick_FiresUpdateNotSelect(t *testing.T) {
	sess, gameRoot := dlssGUIFakes(t, nil)
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
	sess, gameRoot := dlssGUIFakes(t, nil)
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
	sess, gameRoot := dlssGUIFakes(t, gate)
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
	sess, _ := dlssGUIFakes(t, nil)
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
	sess, _ := dlssGUIFakes(t, nil)
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
