package gui

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// scanOneRow scans the fake library and waits for its single row.
func scanOneRow(t *testing.T, sess *ui.Session) ui.GameRow {
	t.Helper()
	sess.Scan(context.Background())
	deadline := time.Now().Add(15 * time.Second)
	for len(sess.VisibleRows()) == 0 && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	rows := sess.VisibleRows()
	if len(rows) != 1 {
		t.Fatalf("scanned rows %d, want 1", len(rows))
	}
	return rows[0]
}

// cardView renders one card of the given row for click tests.
func cardView(m *model, row ui.GameRow) FrameFn {
	return func() {
		Container(Attrs(Viewport), func() {
			m.fitCards(400)
			m.gameCard(row, 0)
		})
	}
}

// TestCardButtonClick_FiresActionNotSelect: clicking a card's button row
// activates the button (QuickInstall, observable via the EAC consent gate)
// and must NOT open the detail panel — the card's own press gesture must
// not steal activation from the button.
func TestCardButtonClick_FiresActionNotSelect(t *testing.T) {
	sess, gameRoot := guiFakes(t)
	writeGUIFile(t, filepath.Join(gameRoot, "start_protected_game.exe"), "EAC")
	row := scanOneRow(t, sess)
	if !row.EAC {
		t.Fatalf("row %+v not flagged EAC; the consent gate cannot prove QuickInstall fired", row)
	}
	m := newModel(Config{Session: sess})

	headlessFrames(t, 400, 800)
	InputState.MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view) // build
	keyFrame(KeyCodeNone, 0, view) // capture rects from the previous frame
	br := m.cardBtnRect
	if br.Size[0] == 0 || br.Size[1] == 0 {
		t.Fatalf("card button rect not recorded: %+v", br)
	}

	clickRect(br, view)

	deadline := time.Now().Add(15 * time.Second)
	for sess.Snapshot().Confirm == nil && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	if sess.Snapshot().Confirm == nil {
		t.Error("button click did not fire QuickInstall (no EAC consent gate); the card swallowed the button's activation")
	}
	if got := sess.Snapshot().Selected; got != "" {
		t.Errorf("button click opened the detail panel (Selected %q), want no selection", got)
	}
	sess.AnswerConfirm(false)
	t.Logf("button click fired the card action without selecting: %+v", br)
}

// TestCardBodyClick_FiresSelect: clicking the card body (outside the button
// row) selects the game and opens the detail panel.
func TestCardBodyClick_FiresSelect(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 400, 800)
	InputState.MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	r := m.cardRect
	if r.Size[0] == 0 || r.Size[1] == 0 {
		t.Fatalf("card rect not recorded: %+v", r)
	}
	// Click the cover area: horizontally centered, above the button row.
	cover := Rect{Origin: Vec2{r.Origin[0], r.Origin[1] + 30}, Size: Vec2{r.Size[0], r.Size[1] / 2}}
	clickRect(cover, view)

	if got := sess.Snapshot().Selected; got != row.InstallDir {
		t.Errorf("card body click Selected %q, want %q", got, row.InstallDir)
	}
	t.Logf("card body click selected %q", row.InstallDir)
}
