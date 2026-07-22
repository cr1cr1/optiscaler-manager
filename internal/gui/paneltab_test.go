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

// Detail-panel Tab continuation tests. User report: "if the side panel is
// opened, the next item to take focus on TAB press should be the first
// focusable element in the sidepanel details". shirei's focusables registry
// is render-ordered, and ALL grid cards register before the panel (the panel
// renders after the content view), so a plain Tab from a focused card walks
// every remaining card and inner control before ever reaching the panel.
// The continuation jumps focus straight to the panel's FIRST focusable in
// render order — its header Close button (seam: m.panelFirstID), which every
// panel renders, so the jump works for clean games with no version dropdown
// too — and Shift+Tab there returns focus to the selected card. With the
// panel closed the Tab walk is untouched and the seam resets to nil.

// seedExternalPanelSession scans a library whose first manual game carries
// an external OptiScaler install, so both its card AND its detail panel
// render the version-dropdown trigger (clean rows render none). Returns the
// session in grid mode and the external game's install dir.
func seedExternalPanelSession(t *testing.T) (*ui.Session, string) {
	t.Helper()
	sess, _ := guiFakes(t)
	var extDir string
	for i := range 2 {
		dir := filepath.Join(t.TempDir(), fmt.Sprintf("Game%02d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// A real exe makes the dir a game (v0.7): empty dirs are refused.
		if err := os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MZGAME"), 0o644); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			// A branded OptiScaler dll makes this manual game external, so
			// its row renders the version dropdown (manual games probe the
			// game dir itself — app.go's manual-root detect).
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
	for _, r := range sess.VisibleRows() {
		if r.InstallDir == extDir {
			if r.Status != domain.StatusExternal {
				t.Fatalf("row %q status %q, want external (the marker must render the dropdown)", extDir, r.Status)
			}
			return sess, extDir
		}
	}
	t.Fatalf("external row %q not among visible rows %v", extDir, sess.VisibleRows())
	return nil, ""
}

// focusSelectedCardWithPanel opens the detail panel for dir and leaves its
// card holding keyboard focus — the gesture state the continuation acts on.
func focusSelectedCardWithPanel(t *testing.T, m *model, sess *ui.Session, dir string) {
	t.Helper()
	sess.Select(dir)
	focusCard(t, m, dir) // builds with the panel open, then focuses the card
	if got := sess.Snapshot().Selected; got != dir {
		t.Fatalf("detail panel not open for %q (Selected %q)", dir, got)
	}
	if m.panelFirstID == nil {
		t.Fatal("panel's first focusable (header Close button) id not captured while the panel is open")
	}
}

// TestPanelTab_CardTabJumpsToPanelFirst: with the detail panel open and the
// SELECTED card focused, Tab jumps focus straight to the panel's first
// focusable instead of walking the card's own inner controls and every
// remaining card. Nothing in the grid may keep focus.
func TestPanelTab_CardTabJumpsToPanelFirst(t *testing.T) {
	sess, extDir := seedExternalPanelSession(t)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	focusSelectedCardWithPanel(t, m, sess, extDir)
	cardDD := m.cardDDTrigger[extDir]
	if cardDD == nil {
		t.Fatalf("external card %q rendered no version-dropdown trigger", extDir)
	}

	keyFrame(KeyTab, 0, m.rootView)      // focused selected card: Tab continues into the panel
	keyFrame(KeyCodeNone, 0, m.rootView) // focus change applies

	if !IdHasFocus(m.panelFirstID) {
		t.Error("Tab from the selected card did not land on the panel's first focusable (its header Close button)")
	}
	if IdHasFocus(cardDD) {
		t.Error("Tab landed on the card's own dropdown trigger; the jump must skip the remaining grid focusables")
	}
	for dir, id := range m.cardIDs {
		if IdHasFocus(id) {
			t.Errorf("card %q holds focus after the panel jump; want the panel's first focusable exclusively", dir)
		}
	}
}

// TestPanelTab_NoPanelKeepsDefaultWalk: with the detail panel CLOSED, Tab
// from a focused card follows the existing render-order registry walk (card
// → its version-dropdown trigger → its buttons → the next card) exactly as
// before — the continuation must not fire.
func TestPanelTab_NoPanelKeepsDefaultWalk(t *testing.T) {
	sess, extDir := seedExternalPanelSession(t)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	focusCard(t, m, extDir) // panel closed
	cardDD := m.cardDDTrigger[extDir]
	if cardDD == nil {
		t.Fatalf("external card %q rendered no version-dropdown trigger", extDir)
	}

	tabFocus(m) // card -> its version-dropdown trigger (default registry walk)

	if !IdHasFocus(cardDD) {
		t.Error("Tab from a focused card with the panel closed did not land on the card's version-dropdown trigger; the default walk changed")
	}
	if m.panelFirstID != nil && IdHasFocus(m.panelFirstID) {
		t.Error("focus jumped to the panel's first focusable with the panel closed; want the default grid walk")
	}
}

// TestPanelTab_ShiftTabFromPanelFirstReturnsToCard: Shift+Tab on the panel's
// first focusable reverses the jump — focus returns to the selected card.
func TestPanelTab_ShiftTabFromPanelFirstReturnsToCard(t *testing.T) {
	sess, extDir := seedExternalPanelSession(t)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	focusSelectedCardWithPanel(t, m, sess, extDir)
	FocusImmediateOn(m.panelFirstID)
	keyFrame(KeyCodeNone, 0, m.rootView) // focus settles on the panel trigger
	if !IdHasFocus(m.panelFirstID) {
		t.Fatal("panel's first focusable did not take focus")
	}

	keyFrame(KeyTab, ModShift, m.rootView) // Shift+Tab: reverse the continuation
	keyFrame(KeyCodeNone, 0, m.rootView)   // focus change applies

	card := m.cardIDs[extDir]
	if card == nil {
		t.Fatalf("selected card %q not in the id registry", extDir)
	}
	if !IdHasFocus(card) {
		t.Error("Shift+Tab from the panel's first focusable did not return focus to the selected card")
	}
	if IdHasFocus(m.panelFirstID) {
		t.Error("panel's first focusable kept focus after Shift+Tab; want the selected card")
	}
}

// TestPanelTab_PanelFirstIsCloseButton: the continuation seam names the
// panel's header Close button — the FIRST focusable in the panel's render
// order (rendered before the version pills/dropdown), so the jump works
// for every panel, clean games included — and never a card's control.
func TestPanelTab_PanelFirstIsCloseButton(t *testing.T) {
	sess, extDir := seedExternalPanelSession(t)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	sess.Select(extDir)
	keyFrame(KeyCodeNone, 0, m.rootView) // panel renders; captures the seam
	keyFrame(KeyCodeNone, 0, m.rootView) // settle

	if m.panelFirstID == nil {
		t.Fatal("open panel captured no first-focusable id (its header Close button did not render?)")
	}
	if m.ddTriggerID == nil {
		t.Fatal("no version-dropdown trigger rendered at all")
	}
	// The panel renders last among dropdown renderers, so ddTriggerID holds
	// the panel's dropdown trigger id at frame end — and the Close button
	// renders BEFORE it, so the seam must NOT be the dropdown trigger.
	if m.panelFirstID == m.ddTriggerID {
		t.Error("panelFirstID == the panel's version-dropdown trigger; the first focusable in render order is the header Close button")
	}
	if cardDD := m.cardDDTrigger[extDir]; cardDD != nil && m.panelFirstID == cardDD {
		t.Error("panelFirstID equals the CARD's dropdown trigger; the seam must capture the panel's own first focusable")
	}
}

// TestPanelTab_CleanGameTabJumpsToPanel: a clean game renders no version
// dropdown, but every panel renders the header Close button — so the Tab
// continuation must still jump into the panel (the reviewer finding: the
// old dropdown-trigger seam left clean games with no jump target at all).
func TestPanelTab_CleanGameTabJumpsToPanel(t *testing.T) {
	sess, extDir := seedExternalPanelSession(t)
	m := newModel(Config{Session: sess})
	// The seed library's other manual game is clean (no OptiScaler pill,
	// hence no version dropdown in its panel).
	cleanDir := ""
	for _, r := range sess.VisibleRows() {
		if r.InstallDir != extDir {
			cleanDir = r.InstallDir
		}
	}
	if cleanDir == "" {
		t.Fatal("no clean game found beside the external one")
	}

	headlessFrames(t, 1200, 700)
	focusSelectedCardWithPanel(t, m, sess, cleanDir) // fatals if panelFirstID is nil

	keyFrame(KeyTab, 0, m.rootView)      // focused selected card: Tab continues into the panel
	keyFrame(KeyCodeNone, 0, m.rootView) // focus change applies

	if !IdHasFocus(m.panelFirstID) {
		t.Error("Tab from a CLEAN game's selected card did not land on the panel's first focusable; the jump must not depend on the version dropdown")
	}
	for dir, id := range m.cardIDs {
		if IdHasFocus(id) {
			t.Errorf("card %q holds focus after the panel jump; want the panel's first focusable exclusively", dir)
		}
	}
}

// TestPanelTab_PanelFirstIDClearsOnClose: the seam resets to nil whenever
// the panel is absent — a stale id must never steer the Tab continuation
// into a detached node on the first frame of a reopen.
func TestPanelTab_PanelFirstIDClearsOnClose(t *testing.T) {
	sess, extDir := seedExternalPanelSession(t)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1200, 700)
	sess.Select(extDir)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.panelFirstID == nil {
		t.Fatal("setup: open panel captured no first-focusable id")
	}

	sess.Select("") // close the panel
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.panelFirstID != nil {
		t.Error("panelFirstID survived the panel closing; a stale id can steer Tab into a detached node on reopen")
	}
	t.Log("panelFirstID reset to nil with the panel closed")
}
