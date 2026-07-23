package gui

import (
	"os"
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
	cardPad    = 10 // inner card padding
	cardGapV   = 8  // vertical gap between components
	cardGapH   = 8  // horizontal gap inside row containers
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
	chrome := badgeRowH + textRowH + 2*pillRowH + buttonRowH + 5*cardGapV
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
// Focus is PER-CARD: the grid region itself is no Tab stop — every card is
// Focusable, and Tab walks card → its version-dropdown trigger → its
// buttons → the next card (shirei's focusables registry is render-ordered).
func (m *model) gridView() {
	rows := m.visibleRows()
	m.cardRingClip = Rect{}
	m.gridCursorRect = Rect{}
	if len(rows) == 0 {
		m.emptyState()
		return
	}
	cols := m.cols
	if cols < 1 {
		cols = 1
	}
	chunks := chunkRows(rows, cols)
	// One-ring rule: the selIdx cursor ring is suppressed while any card
	// holds keyboard focus (the focused card wears the ring instead). The
	// check uses LAST frame's id registry — the fresh one is rebuilt below
	// as the visible cards render.
	m.gridCardFocused = false
	for _, id := range m.cardIDs {
		if IdHasFocus(id) {
			m.gridCardFocused = true
			break
		}
	}
	// The one-ring rule covers grid DESCENDANTS too: a focused inner
	// control (button, dropdown trigger) wears its own ring, so the
	// selIdx cursor ring must stay off. cardIDs holds only card
	// containers, so the check falls back to last frame's per-card
	// focus-within record (same staleness contract as the id registry).
	if !m.gridCardFocused {
		for _, within := range m.cardFocusWithin {
			if within {
				m.gridCardFocused = true
				break
			}
		}
	}
	m.gridFocusWithin = false
	// The id registries rebuild every grid frame: shirei identities are
	// path-scoped, so the detail panel re-nesting the grid invalidates them.
	m.cardIDs = make(map[string]ContainerId, len(rows))
	m.cardDDTrigger = make(map[string]ContainerId, len(rows))
	m.cardFocusWithin = make(map[string]bool, len(rows))
	m.gridRows = rows
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
			if m.scrollCursorPending {
				m.scrollCursorPending = false
				c := m.cols
				if c < 1 {
					c = 1
				}
				VirtualListScrollIntoView("grid", m.selIdx/c)
			}
			// 1px vertical padding: shirei draws a container's border
			// straddling its edge (half the stroke outside the rect), and
			// this row's Clip used to sit flush against the card, scissoring
			// the outer half of the top/bottom focus ring (the visible
			// "thin ring" bug — the sides survived because rowPadH/cardGap
			// already gave them room). 1px of row padding clears the stroke
			// without touching card geometry; the chunk item's 8px height
			// slack absorbs the taller row.
			Container(Attrs(Row, Gap(cardGap), Pad2(1, rowPadH), MinSize(w, float32(m.cardH)), Clip), func() {
				m.rowClipRect = GetScreenRectOf(CurrentId())
				for j := range chunks[i] {
					m.gameCard(chunks[i][j], i*cols+j)
				}
			})
		})
}

// gameCard renders one cover card: platform pill, status badges, cover,
// title, version pills, tech pills, and the install/launch buttons. Hover
// lifts the card with an accent border and a soft shadow and records the
// hovered game on the model. The card itself is the focus stop (Focusable +
// CycleFocusOnTab + FocusOnClick): click AND Tab focus it, and Tab then
// walks its inner focusables in render order (version dropdown, buttons).
// The card wears the focus ring when it holds focus, or when it is the
// keyboard cursor (idx == selIdx) while no card is focused — one ring only
// (cursor wins over hover; both are 1.5px borders, so the ring never
// shifts card geometry).
func (m *model) gameCard(e ui.GameRow, idx int) {
	cardW, cardH := m.cardW, m.cardH
	coverW := float32(cardW - 2*cardPad)
	m.tierPillRect = Rect{}
	m.ddTriggerID = nil
	m.ddFocusRing = false
	Container(Attrs(Focusable, Pad(cardPad), Gap(cardGapV), FixSize(float32(cardW), float32(cardH)), BackgroundVec(bgCard), Corners(radiusM), Clip), func() {
		m.lastRenderedIdx = idx
		if m.cardIDs != nil {
			m.cardIDs[e.InstallDir] = CurrentId()
		}
		// Record whether keyboard focus sits anywhere inside THIS card
		// (the card itself or an inner control): next frame's one-ring
		// check widens gridCardFocused with it, and the global arrow
		// handler releases an inner control's focus on a cursor move
		// only when focus is inside the grid (gridFocusWithin).
		within := HasFocusWithin()
		if m.cardFocusWithin != nil {
			m.cardFocusWithin[e.InstallDir] = within
		}
		if within {
			m.gridFocusWithin = true
		}
		// Deferred re-assert: clicks and Enter set cardFocusPending because
		// opening/closing the detail panel re-nests the grid — shirei
		// identities are path-scoped, so focus grabbed mid-gesture would be
		// orphaned. Re-assert once, on this card's fresh identity, the first
		// frame the card re-renders (mirrors listFocusPending, per-card). A
		// card scrolled away simply re-asserts when it next renders.
		if m.cardFocusPending == e.InstallDir {
			m.cardFocusPending = ""
			FocusImmediateOn(CurrentId())
		}
		CycleFocusOnTab()
		FocusOnClick()
		// Cursor follows focus: Tabbing onto a card moves the keyboard
		// cursor onto it, so focus and cursor can never diverge. Click and
		// arrow paths set selIdx directly too — ReceivedFocusNow fires on any
		// focus grab (Tab, click, same-frame re-assert), but always writes the
		// identical value those paths already set, so there is no race.
		if ReceivedFocusNow() {
			m.selIdx = idx
			c := m.cols
			if c < 1 {
				c = 1
			}
			VirtualListScrollIntoView("grid", idx/c)
		}
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
		// The ring: focusBorder on the focused card, or on the cursor card
		// while no card is focused (unfocused global-arrow nav keeps its
		// cursor visual). This ModAttrs runs after the hover one, so when
		// both land on one card the ring wins (a focused cursor must stay
		// visible under the pointer). BorderWidth matches hover's 1.5 so the
		// ring never shifts card layout. Grid cards deliberately get no
		// selBg band: the docked detail panel is the selection indicator in
		// grid mode.
		focused := HasFocus()
		if focused || (idx == m.selIdx && !m.gridCardFocused && FrameInput.Mouse != MouseClick) {
			m.gridCursorRect = m.cardRect
			m.cardRingClip = m.rowClipRect
			ModAttrs(func(a *AttrSet) {
				a.BorderWidth = 1.5
				a.BorderColor = focusBorder
			})
		}
		// A focused card owns the arrows and Enter (consumed so the global
		// fallback at frame end cannot double-fire): arrows move the cursor
		// via the shared moveGridSel AND hand focus to the new cursor card;
		// Enter toggles the detail panel for the focused==cursor card.
		if focused && len(m.gridRows) > 0 {
			switch FrameInput.Key {
			case KeyRight:
				m.moveGridSel(m.gridRows, 1)
				FrameInput.Key = KeyCodeNone
				m.focusCursorCard()
			case KeyLeft:
				m.moveGridSel(m.gridRows, -1)
				FrameInput.Key = KeyCodeNone
				m.focusCursorCard()
			case KeyDown:
				m.moveGridSel(m.gridRows, m.cols)
				FrameInput.Key = KeyCodeNone
				m.focusCursorCard()
			case KeyUp:
				m.moveGridSel(m.gridRows, -m.cols)
				FrameInput.Key = KeyCodeNone
				m.focusCursorCard()
			case KeyEnter:
				m.toggleListDetail(m.gridRows)
				FrameInput.Key = KeyCodeNone
				m.cardFocusPending = m.gridRows[m.selIdx].InstallDir
				m.scrollCursorPending = true
			}
		}
		Container(Attrs(Row, Gap(cardGapH)), func() {
			if e.Platform != "" {
				badgePill(e.Platform, ui.ToneGray)
			}
			if e.EAC {
				badgePill("EAC", ui.ToneRed)
			}
			if b, ok := statusPill(&e); ok {
				badgePill(b.Label, b.Tone)
			}
			m.protonTierPill(e.ProtonTier)
			if m.sess != nil && m.sess.OpBusy(e.InstallDir) {
				spinnerGlyph()
			}
		})
		m.coverArt(e, coverW, coverW*coverRatio)
		txt(e.Title)
		if pills := versionPills(&e); len(pills) > 0 {
			Container(Attrs(Row, Gap(cardGapH)), func() {
				start := 0
				// The OptiScaler pill is the version dropdown; component and
				// Proton pills stay static.
				if b, ok := optiBadge(&e); ok {
					m.versionDropdown(&e, b.Label, b.Tone)
					start = 1
				}
				for _, p := range pills[start:] {
					badgePill(p.Label, p.Tone)
				}
			})
		}
		// versionDropdown left this card's trigger in ddTriggerID (the next
		// card resets it); capture it per install dir for the Tab-order seam.
		if m.cardDDTrigger != nil && m.ddTriggerID != nil {
			m.cardDDTrigger[e.InstallDir] = m.ddTriggerID
		}
		if len(e.TechBadges) > 0 {
			Container(Attrs(Row, Gap(cardGapH)), func() {
				for _, b := range e.TechBadges {
					badgePill(b.Label, b.Tone)
				}
			})
		}
		// Buttons pin to the card bottom so cards with and without pill rows
		// align exactly.
		Filler(1)
		var btnRowID ContainerId
		if m.sess != nil {
			Container(Attrs(Row, Gap(cardGapH)), func() {
				m.cardBtnRect = GetScreenRectOf(CurrentId())
				if focusableButton(SymIRight, quickLabel(&e)) {
					m.sess.QuickInstall(e.InstallDir)
				}
				if launchable(&e) && focusableButton(0, "Launch") {
					m.launchGame(e)
				}
				m.cardLastButtonID = GetLastId()
			})
			btnRowID = GetLastId()
		}
		// shirei has a single global active node: on mouse-down over a button the
		// card's PressAction is built last and would steal activation, swallowing
		// the button and opening the detail panel instead. While the pointer is
		// over the button row or the version-dropdown trigger the card skips its
		// own press gesture entirely.
		overButtons := btnRowID != nil && IdIsHovered(btnRowID)
		overDropdown := m.ddTriggerID != nil && IdIsHovered(m.ddTriggerID)
		if !overButtons && !overDropdown && PressAction() && m.sess != nil {
			// A card click is also a cursor move. The focus grab itself is
			// FocusOnClick's job (mouse-down frame); cardFocusPending
			// re-asserts it after the panel re-nests the grid (see above).
			m.selIdx = idx
			m.cardFocusPending = e.InstallDir
			m.sess.Select(e.InstallDir)
		}
	})
}

// coverArt renders the game's cover scaled into the cover box. The covers
// package hands back a tiny dark placeholder file when a game has no art;
// that counts as "no cover" and gets the gradient placeholder instead. A
// path whose file was deleted (e.g. the user wiped the cover cache) must
// not reach shirei's image loader — it nil-dereferences on missing files.
func (m *model) coverArt(e ui.GameRow, w, h float32) {
	if e.CoverPath != "" && !isPlaceholderCover(e.CoverPath) {
		if _, err := os.Stat(e.CoverPath); err == nil {
			Image(e.CoverPath, Vec2{w, h})
			return
		}
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
