package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// pillWrapRows returns two grid rows: a plain upstream install whose pills
// fit on one line, and a fork install with a long distribution name plus
// component/Proton pills that cannot fit on one card-width line.
func pillWrapRows() []ui.GameRow {
	return []ui.GameRow{
		{Title: "Plain Game", AppID: "1", InstallDir: "/g/plain", Status: domain.StatusCommitted,
			OptiScalerVersion: "0.9.4"},
		{Title: "Fork Game", AppID: "2", InstallDir: "/g/fork", Status: domain.StatusCommitted,
			Fork: "jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass", OptiScalerVersion: "0.8.92",
			Components: []string{"DLSS 3.7.20", "FSR 3.1", "XeSS 2.0"}, CompatPrefix: "/pfx"},
	}
}

// wrapLineCount must mirror shirei's greedy wrap packing: a pill wraps only
// when it strictly exceeds the remaining line width.
func TestWrapLineCount(t *testing.T) {
	cases := []struct {
		name   string
		widths []float32
		want   int
	}{
		{"empty", nil, 0},
		{"single fits", []float32{50}, 1},
		{"two fit exactly", []float32{50, 42}, 1}, // 50+8+42 == 100, not over
		{"two just over", []float32{50, 43}, 2},   // 50+8+43 > 100
		{"two wide", []float32{60, 60}, 2},        // 60+8+60 > 100
		{"four pack two lines", []float32{30, 30, 30, 30}, 2},
		{"wider than line alone", []float32{120}, 1}, // first on a line never wraps
		{"wide then small", []float32{120, 10}, 2},   // second does not fit after the wide one
		{"three lines", []float32{60, 60, 60}, 3},    // no two fit together
	}
	for _, c := range cases {
		if got := wrapLineCount(c.widths, 100, 8); got != c.want {
			t.Errorf("%s: wrapLineCount(%v) = %d, want %d", c.name, c.widths, got, c.want)
		}
	}
}

// cardPillLines estimates the wrapped line counts of a card's pill rows from
// the shaped label widths: the long fork pill row must estimate 2+ lines at
// the medium card width while the plain row stays on one.
func TestCardPillLinesLongForkWraps(t *testing.T) {
	m := newModel(Config{})
	rows := pillWrapRows()
	availW := float32(cardSizeForPreset(m.cardSize) - 2*cardPad)

	if _, v, _ := m.cardPillLines(&rows[0], availW); v != 1 {
		t.Errorf("plain row version pill lines = %d, want 1", v)
	}
	if _, v, _ := m.cardPillLines(&rows[1], availW); v < 2 {
		t.Errorf("fork row version pill lines = %d, want >= 2 (long fork pill must wrap)", v)
	}
}

// Pills wrap inside the card instead of overflowing it, and every card in
// the grid grows to the same height — the one the worst pill stack needs —
// so a wrapped card never clips its pill row.
func TestGridUniformCardHeightWithWrappedPills(t *testing.T) {
	m := newModel(Config{})
	m.state = ui.State{Mode: ui.ViewGrid, StatusLine: "2 games", Rows: pillWrapRows()}

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // settle

	base := cardContentH(cardSizeForPreset(m.cardSize))
	if m.cardH <= base {
		t.Errorf("cardH = %d, want > %d: the wrapped fork pills must grow EVERY card", m.cardH, base)
	}
	row := m.versionPillRowRect
	if row.Size[1] <= float32(pillRowH+4) {
		t.Errorf("version pill row height = %v, want a wrapped row (> %d)", row.Size[1], pillRowH+4)
	}
	card := m.cardRect // last rendered card = the fork card
	if got := card.Size[1]; got != float32(m.cardH) {
		t.Errorf("rendered card height = %v, want the shared cardH %d", got, m.cardH)
	}
	if rowBottom, cardBottom := row.Origin[1]+row.Size[1], card.Origin[1]+card.Size[1]; rowBottom > cardBottom+0.5 {
		t.Errorf("pill row bottom %v past card bottom %v: pills clipped by the card", rowBottom, cardBottom)
	}
}

// The detail panel's version pills wrap too: the long fork pill plus the
// component pills must not run past the panel's edge.
func TestDetailPanelPillsWrap(t *testing.T) {
	m := newModel(Config{})
	rows := pillWrapRows()
	m.state = ui.State{Mode: ui.ViewGrid, StatusLine: "2 games", Rows: rows, Selected: rows[1].InstallDir}

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // settle

	row := m.versionPillRowRect // the panel renders after the grid and captures last
	if row.Size[1] <= float32(pillRowH+4) {
		t.Errorf("detail panel version pill row height = %v, want a wrapped row (> %d)", row.Size[1], pillRowH+4)
	}
}
