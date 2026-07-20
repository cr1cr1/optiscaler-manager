package gui

import (
	"path/filepath"
	"strings"

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
	cardPad    = 6  // inner card padding
)

// Fixed card chrome below the cover: badge row, title, two pill rows, and
// the button row, each one text line tall, plus gaps and padding.
const (
	badgeRowH  = 18
	textRowH   = 18
	pillRowH   = 18
	buttonRowH = 30
)

// cardContentH sizes a card so every element fits: badge row, cover,
// title, version pills, tech pills, and the button row, plus gaps.
func cardContentH(cardW int) int {
	coverH := int(float32(cardW-2*cardPad) * coverRatio)
	chrome := badgeRowH + textRowH + 2*pillRowH + buttonRowH + 5*int(sp4) + 2*cardPad
	return coverH + chrome
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

// gridItemCount adds a trailing spacer row to the chunk count so the last
// card row never renders flush against the viewport edge.
func gridItemCount(chunks int) int { return chunks + 1 }

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
	VirtualListView("grid", gridItemCount(len(chunks)),
		func(i int) any {
			if i == len(chunks) {
				return "spacer"
			}
			return i
		},
		func(i int, w float32) float32 {
			if i == len(chunks) {
				return sp24
			}
			return float32(m.cardH) + 8
		},
		func(i int, w float32) {
			if i == len(chunks) {
				return
			}
			m.fitCards(int(w))
			Container(Attrs(Row, Gap(cardGap), Pad2(0, rowPadH), MinSize(w, float32(m.cardH)), Clip), func() {
				for j := range chunks[i] {
					m.gameCard(chunks[i][j])
				}
			})
		})
}

// gameCard renders one cover card: platform pill, status badges, cover,
// title, version pills, tech pills, and the install/launch buttons. Hover
// lifts the card with an accent border and a soft shadow and records the
// hovered game on the model.
func (m *model) gameCard(e ui.GameRow) {
	cardW, cardH := m.cardW, m.cardH
	coverW := float32(cardW - 2*cardPad)
	Container(Attrs(Pad(cardPad), Gap(sp4), FixSize(float32(cardW), float32(cardH)), BackgroundVec(bgCard), Corners(radiusM), Clip), func() {
		if IsHovered() {
			m.hoveredDir = e.InstallDir
			ModAttrs(func(a *AttrSet) {
				a.BorderWidth = 1.5
				a.BorderColor = accent
				a.Blur = 16
				a.Alpha = 0.3
				a.Offset[1] = 2
			})
		} else if m.hoveredDir == e.InstallDir {
			m.hoveredDir = ""
		}
		m.cardRect = GetScreenRectOf(CurrentId())
		Container(Attrs(Row, Gap(sp4)), func() {
			if e.Platform != "" {
				badgePill(e.Platform, ui.ToneGray)
			}
			if e.EAC {
				badgePill("EAC", ui.ToneRed)
			}
			if e.Actionable {
				badgePill(string(e.Status), ui.ToneRed)
			}
			if m.sess != nil && m.sess.OpBusy(e.InstallDir) {
				spinnerGlyph()
			}
		})
		m.coverArt(e, coverW, coverW*coverRatio)
		txt(e.Title)
		if pills := versionPills(&e); len(pills) > 0 {
			Container(Attrs(Row, Gap(sp4)), func() {
				for _, p := range pills {
					badgePill(p.Label, p.Tone)
				}
			})
		}
		if len(e.TechBadges) > 0 {
			Container(Attrs(Row, Gap(sp4)), func() {
				for _, b := range e.TechBadges {
					badgePill(b.Label, b.Tone)
				}
			})
		}
		if m.sess != nil {
			Container(Attrs(Row, Gap(sp4)), func() {
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

// coverArt renders the game's cover scaled into the cover box. The covers
// package hands back a tiny dark placeholder file when a game has no art;
// that counts as "no cover" and gets the gradient placeholder instead.
func (m *model) coverArt(e ui.GameRow, w, h float32) {
	if e.CoverPath != "" && !isPlaceholderCover(e.CoverPath) {
		Image(e.CoverPath, Vec2{w, h})
		return
	}
	coverPlaceholder(e.Title, w, h)
}

// isPlaceholderCover reports whether path is the covers package's generated
// no-art placeholder.
func isPlaceholderCover(path string) bool {
	return filepath.Base(path) == "_placeholder.png"
}

// coverPlaceholder renders a deterministic gradient tile for games without
// cover art: the hue comes from a title hash, with a centered image glyph
// and the title initial.
func coverPlaceholder(title string, w, h float32) {
	hue := float32(fnv32(title) % 360)
	Container(Attrs(FixSize(w, h), Background(hue, 32, 26, 1), GradVec(Vec4{0, 12, 24, 0}), Corners(radiusS), Center, Gap(sp4)), func() {
		Icon(TypImage, FontSize(28), TextColor(hue, 25, 72, 1))
		if initial := titleInitial(title); initial != "" {
			Label(initial, FontSize(15), TextColor(hue, 20, 85, 1), FontWeight(WeightBold))
		}
	})
}

// fnv32 hashes a title so each game's placeholder lands on a stable hue.
func fnv32(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// titleInitial is the uppercased first letter of a game title.
func titleInitial(s string) string {
	for _, r := range s {
		return strings.ToUpper(string(r))
	}
	return ""
}
