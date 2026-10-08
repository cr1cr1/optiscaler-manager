package gui

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"
)

// TestGridClickLastRowKeepsScroll (issue 037): clicking a card on the
// grid's last, partially-filled row opens the detail panel without
// scrolling the card view back to the top. Opening the panel re-nests
// the grid (shirei identities are path-scoped), which recreates the
// virtual list's node at scroll offset 0; the keyboard Enter path arms
// the deferred scroll-into-view restore (scrollCursorPending), and the
// click path must arm it too — the identity-churn fallback only fires
// when the cursor card happens to render inside the reset (top) window,
// which a deep, last-row card never does.
func TestGridClickLastRowKeepsScroll(t *testing.T) {
	sess, gameRoot := guiFakes(t)
	// 13 more games (14 total): enough rows to scroll, with a partial
	// last row at the test window's column count.
	steamRoot := filepath.Dir(filepath.Dir(filepath.Dir(gameRoot)))
	for i := 2; i <= 14; i++ {
		writeGUIFile(t, filepath.Join(steamRoot, "steamapps", fmt.Sprintf("appmanifest_%d.acf", 100+i)),
			fmt.Sprintf(`"AppState" { "appid" "%d" "name" "Game %d" "installdir" "Game%d" }`, 100+i, i, i))
		writeGUIFile(t, filepath.Join(steamRoot, "steamapps", "common", fmt.Sprintf("Game%d", i), "bin", "game.exe"), "GAME")
	}
	sess.Scan(context.Background())
	rows := waitScanSettled(t, sess, 14)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 1100, 800)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	cols := m.cols
	if cols < 2 {
		t.Fatalf("cols %d: the test needs a real multi-column grid", cols)
	}
	if len(rows)%cols == 0 {
		t.Fatalf("%d games fill %d columns exactly — no partial last row; retune the window", len(rows), cols)
	}

	// Scroll the last row into view (minimal alignment → bottom edge).
	VirtualListScrollIntoView("grid", (len(rows)-1)/cols)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	last := rows[len(rows)-1]
	if m.cardIDs[last.InstallDir] == nil {
		t.Fatalf("last-row card %q not visible after scroll-into-view; test setup broken", last.InstallDir)
	}
	// The last rendered card is the final card of the last row — the
	// spacer item after it carries no card.
	card := m.cardRect
	if card.Size[0] == 0 || card.Size[1] == 0 {
		t.Fatalf("last-row card rect unresolved: %+v", card)
	}

	clickRect(card, m.rootView)
	if got := sess.Snapshot().Selected; got != last.InstallDir {
		t.Fatalf("click Selected %q, want %q", got, last.InstallDir)
	}

	// Settle: the panel opens, the grid re-nests, the restore fires.
	for i := 0; i < 5; i++ {
		keyFrame(KeyCodeNone, 0, m.rootView)
	}
	if m.cardIDs[last.InstallDir] == nil {
		t.Errorf("clicked last-row card %q is no longer rendered after the panel opened — the grid scrolled back to the top", last.InstallDir)
	}
}
