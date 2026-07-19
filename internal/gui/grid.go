package gui

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Card geometry: cards adapt to the live list width so narrow windows
// (tiling WMs) never overflow horizontally; ultrawide windows cap columns
// and card width instead of stretching cards absurdly.
const (
	cardGap    = 10
	targetCard = 200 // px; cols = width/targetCard
	coverRatio = 1.5 // 600x900 covers are 2:3
	maxCols    = 8
	maxCardW   = 320
	rowPadH    = 12 // horizontal padding each side of a grid row
)

// cardContentH sizes a card so every element fits: badge row, cover,
// title, version pills, tech pills, and the button row, plus gaps.
func cardContentH(cardW int) int {
	coverH := int(float32(cardW-12) * coverRatio)
	return coverH + 140
}

// chunkRows groups rows into rows-of-cols for the virtualized grid. cols is
// clamped to ≥1.
func chunkRows(rows []ui.GameRow, cols int) [][]ui.GameRow {
	if cols < 1 {
		cols = 1
	}
	var chunks [][]ui.GameRow
	for i := 0; i < len(rows); i += cols {
		end := i + cols
		if end > len(rows) {
			end = len(rows)
		}
		chunks = append(chunks, rows[i:end])
	}
	return chunks
}

// fitCards derives columns and card size from the live row width: at least
// one column, at most maxCols, and cards never wider than maxCardW (rows
// stay left-aligned on ultrawide windows).
func (m *model) fitCards(w int) {
	inner := w - 2*rowPadH
	cols := inner / targetCard
	if cols < 1 {
		cols = 1
	}
	if cols > maxCols {
		cols = maxCols
	}
	m.cols = cols
	m.cardW = (inner - (cols-1)*cardGap) / cols
	if m.cardW > maxCardW {
		m.cardW = maxCardW
	}
	m.cardH = cardContentH(m.cardW)
}

// gridView is the cover-card grid (the reference client's main view).
// Columns and card size are recomputed from the live width each frame.
func (m *model) gridView() {
	rows := m.visibleRows()
	if len(rows) == 0 {
		m.emptyState()
		return
	}
	cols := m.cols
	if cols < 1 {
		cols = 1
	}
	chunks := chunkRows(rows, cols)
	VirtualListView("grid", len(chunks),
		func(i int) any { return i },
		func(i int, w float32) float32 { return float32(m.cardH) + 8 },
		func(i int, w float32) {
			m.fitCards(int(w))
			Container(Attrs(Row, Gap(cardGap), Pad2(0, rowPadH), MinSize(w, float32(m.cardH)), Clip), func() {
				for j := range chunks[i] {
					m.gameCard(chunks[i][j])
				}
			})
		})
}

// gameCard renders one cover card: platform pill, status badges, cover,
// title, version pills, tech pills, and the install/launch buttons.
func (m *model) gameCard(e ui.GameRow) {
	cardW, cardH := m.cardW, m.cardH
	coverW := float32(cardW - 12)
	Container(Attrs(Pad(6), Gap(4), FixSize(float32(cardW), float32(cardH)), BackgroundVec(bgCard), Corners(6), Clip), func() {
		Container(Attrs(Row, Gap(4)), func() {
			if e.Platform != "" {
				badgePill(e.Platform, ui.ToneGray)
			}
			if e.EAC {
				badgePill("EAC", ui.ToneRed)
			}
			if e.Actionable {
				badgePill(string(e.Status), ui.ToneRed)
			}
		})
		if e.CoverPath != "" {
			Image(e.CoverPath, Vec2{coverW, coverW * coverRatio})
		} else {
			Container(Attrs(FixSize(coverW, coverW*coverRatio), Background(230, 10, 30, 1)), func() {})
		}
		txt(e.Title)
		if pills := versionPills(&e); len(pills) > 0 {
			Container(Attrs(Row, Gap(4)), func() {
				for _, p := range pills {
					badgePill(p.Label, p.Tone)
				}
			})
		}
		if len(e.TechBadges) > 0 {
			Container(Attrs(Row, Gap(4)), func() {
				for _, b := range e.TechBadges {
					badgePill(b.Label, b.Tone)
				}
			})
		}
		if m.sess != nil {
			Container(Attrs(Row, Gap(4)), func() {
				if focusableButton(SymIRight, quickLabel(&e)) {
					m.sess.QuickInstall(e.InstallDir)
				}
				if launchable(&e) && focusableButton(0, "Launch") {
					m.launchGame(e)
				}
			})
		}
		if PressAction() && m.sess != nil {
			m.sess.Select(e.InstallDir)
		}
	})
}
