package gui

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Card geometry (cover is 600×900 aspect = 2:3).
const (
	cardWidth  = 190
	cardHeight = 330
	coverW     = 170
	coverH     = 255
)

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
// Cards flow cols-per-row; cols is recomputed from the live width each frame.
func (m *model) gridView() {
	rows := m.visibleRows()
	cols := m.cols
	if cols < 1 {
		cols = 1
	}
	chunks := chunkRows(rows, cols)
	VirtualListView("grid", len(chunks),
		func(i int) any { return i },
		func(i int, w float32) float32 { return cardHeight },
		func(i int, w float32) {
			if c := int(w) / cardWidth; c >= 1 && c != m.cols {
				m.cols = c
			}
			Container(Attrs(Row, Gap(10), Pad2(0, 6), MinSize(w, cardHeight)), func() {
				for j := range chunks[i] {
					m.gameCard(chunks[i][j])
				}
			})
		})
}

// gameCard renders one cover card: platform pill, installed badge, cover,
// title, tech pills, and the quick-install toggle.
func (m *model) gameCard(e ui.GameRow) {
	Container(Attrs(Pad(6), Gap(4), FixSize(cardWidth, cardHeight), BackgroundVec(bgCard), Corners(6)), func() {
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
			Image(e.CoverPath, Vec2{coverW, coverH})
		} else {
			Container(Attrs(FixSize(coverW, coverH), Background(230, 10, 30, 1)), func() {})
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
