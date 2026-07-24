package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "go.hasen.dev/shirei"
)

// --- Card size presets ---

func TestCardSizePresets_Valid(t *testing.T) {
	want := map[string]int{"small": 200, "medium": 240, "large": 280}
	for size, px := range want {
		got := cardSizeForPreset(size)
		if got != px {
			t.Errorf("cardSizeForPreset(%q) = %d, want %d", size, got, px)
		}
	}
	if got := cardSizeForPreset("bogus"); got != cardSizeForPreset("medium") {
		t.Errorf("cardSizeForPreset(bogus) = %d, want medium fallback %d", got, cardSizeForPreset("medium"))
	}
}

func TestFitCards_TargetDrivesColumnCount(t *testing.T) {
	m := newModel(Config{})
	m.fitCards(1100)
	colsMedium := m.cols
	m.cardSize = "large"
	m.fitCards(1100)
	colsLarge := m.cols
	if colsLarge >= colsMedium {
		t.Errorf("larger preset: cols %d >= medium cols %d, want fewer", colsLarge, colsMedium)
	}
	m.cardSize = "small"
	m.fitCards(1100)
	colsSmall := m.cols
	if colsSmall <= colsMedium {
		t.Errorf("smaller preset: cols %d <= medium cols %d, want more", colsSmall, colsMedium)
	}
}

func TestFitCards_FillsWidth(t *testing.T) {
	m := newModel(Config{})
	for _, w := range []int{400, 800, 1100, 1600} {
		m.fitCards(w)
		// cardW must be exactly the preset, never stretched.
		if m.cardW != cardSizeForPreset(m.cardSize) {
			t.Errorf("w=%d: cardW=%d, want fixed preset %d", w, m.cardW, cardSizeForPreset(m.cardSize))
		}
		// At least 1 column; columns fill the width via Filler spacers.
		if m.cols < 1 {
			t.Errorf("w=%d: cols=%d, want >=1", w, m.cols)
		}
	}
}

func TestFitCards_PanelOpenDropsColumns(t *testing.T) {
	m := newModel(Config{})
	m.fitCards(1100)
	colsBefore := m.cols
	panelW := int(detailPanelWidth(1100))
	m.fitCards(1100 - panelW)
	if m.cols >= colsBefore {
		t.Errorf("panel open: cols %d -> %d, want fewer columns", colsBefore, m.cols)
	}
}

// --- List row non-overlap ---

func TestListRows_DoNotOverlap(t *testing.T) {
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		dir := filepath.Join(t.TempDir(), name)
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MZGAME"), 0o644)
		sess.AddDirectory(dir)
	}
	sess.Scan(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for len(sess.VisibleRows()) < 5 && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	if len(sess.VisibleRows()) < 5 {
		t.Fatalf("rows %d, want >=5", len(sess.VisibleRows()))
	}

	headlessFrames(t, 400, 800)
	sess.ToggleView()
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)

	if len(m.listRowRects) < 2 {
		t.Fatalf("listRowRects = %d, want >=2 for overlap check", len(m.listRowRects))
	}
	for i := 0; i+1 < len(m.listRowRects); i++ {
		if m.listRowRects[i].Size[1] == 0 {
			continue
		}
		bottom := m.listRowRects[i].Origin[1] + m.listRowRects[i].Size[1]
		nextTop := m.listRowRects[i+1].Origin[1]
		if nextTop < bottom {
			t.Errorf("rows %d-%d overlap: row %d bottom=%.1f, row %d top=%.1f (overlap=%.1fpx)",
				i, i+1, i, bottom, i+1, nextTop, bottom-nextTop)
		}
	}
}
