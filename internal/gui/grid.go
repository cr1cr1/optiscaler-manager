package gui

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Card geometry: cards adapt to the live list width so narrow windows
// (tiling WMs) never overflow horizontally.
const (
	cardGap    = 10
	targetCard = 200 // px; cols = width/targetCard
	coverRatio = 1.5 // 600x900 covers are 2:3
)

// cardContentH sizes a card so every element fits: badge row, cover,
// title, tech pills, and the quick-install button, plus gaps.
func cardContentH(cardW int) int {
	coverH := int(float32(cardW-12) * coverRatio)
	return coverH + 118
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

// gridView is the cover-card grid (the reference client's main view).
// Columns and card size are recomputed from the live width each frame.
func (m *model) gridView() {
	rows := m.visibleRows()
	cols := m.cols
	if cols < 1 {
		cols = 1
	}
	chunks := chunkRows(rows, cols)
	VirtualListView("grid", len(chunks),
		func(i int) any { return i },
		func(i int, w float32) float32 { return float32(m.cardH) + 8 },
		func(i int, w float32) {
			if c := int(w) / targetCard; c >= 1 && c != m.cols {
				m.cols = c
			}
			if m.cols > 0 {
				m.cardW = (int(w) - (m.cols-1)*cardGap) / m.cols
				m.cardH = cardContentH(m.cardW)
			}
			Container(Attrs(Row, Gap(cardGap), Pad2(0, 12), MinSize(w, float32(m.cardH)), Clip), func() {
				for j := range chunks[i] {
					m.gameCard(chunks[i][j])
				}
			})
		})
}

// gameCard renders one cover card: platform pill, installed badge, cover,
// title, tech pills, and the quick-install toggle.
func (m *model) gameCard(e ui.GameRow) {
	cardW, cardH := m.cardW, m.cardH
	coverW := float32(cardW - 12)
	Container(Attrs(Pad(6), Gap(4), FixSize(float32(cardW), float32(cardH)), BackgroundVec(bgCard), Corners(6), Clip), func() {
		Container(Attrs(Row, Gap(4)), func() {
			if e.Platform != "" {
				badgePill(e.Platform, ui.ToneGray)
			}
			if e.Status == domain.StatusCommitted {
				badgePill("✦ OptiScaler", ui.TonePurple)
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
		if len(e.TechBadges) > 0 {
			Container(Attrs(Row, Gap(4)), func() {
				for _, b := range e.TechBadges {
					badgePill(b.Label, b.Tone)
				}
			})
		}
		if m.sess != nil {
			if Button(SymIRight, quickLabel(&e)) {
				m.sess.QuickInstall(e.InstallDir)
			}
		}
		if PressAction() && m.sess != nil {
			m.sess.Select(e.InstallDir)
		}
	})
}
