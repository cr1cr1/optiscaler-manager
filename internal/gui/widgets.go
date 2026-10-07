package gui

import (
	"strings"
	"time"
	"unicode"

	. "go.hasen.dev/shirei"
	"go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
	"github.com/cr1cr1/optiscaler-manager/internal/version"
)

// focusableButton renders widgets.ButtonExt: since shirei v0.6.10 the
// button face itself joins the tab ring, takes focus on click, and
// activates on Space/Enter (armed on press, Clicked on release — cancelled
// if focus moves first). The wrapper stays only for geometry and the focus
// ring; activating it twice (wrapper + face) would double-fire.
func focusableButton(icon widgets.IconGlyph, label string) bool {
	return focusableButtonExt(label, widgets.ButtonAttrs{Icon: icon})
}

// focusableButtonExt is focusableButton with full ButtonAttrs control
// (disabled state, accent, sizing). The returned bool is the completed
// click: pointer release while hovered, or Space/Enter release while
// focused.
func focusableButtonExt(label string, attrs widgets.ButtonAttrs) bool {
	var activated bool
	Container(Attrs(PointerHand, Corners(6)), func() {
		// The face (the tab stop) holds focus; the ring paints on its
		// enclosing wrapper. Attrs must change BEFORE children (shirei
		// panics otherwise), so the check reads the face id recorded on the
		// previous frame through the per-node hook slot.
		prev := Use[ContainerId]("focusable-button-face")
		if *prev != nil && IdHasFocus(*prev) {
			ModAttrs(func(a *AttrSet) {
				a.BorderWidth = 2
				a.BorderColor = focusBorder
			})
		}
		activated = widgets.ButtonExt(label, attrs, widgets.DefaultButtonLook())
		*prev = GetLastId()
	})
	return activated
}

// spinnerFrames is the hand-rolled busy indicator cycle (shirei has no
// spinner widget).
var spinnerFrames = []string{"◐", "◓", "◑", "◒"}

type spinnerState struct {
	idx  int
	last time.Time
}

// spinnerGlyph renders the cycling busy glyph, advancing every 150ms while
// it is on screen.
func spinnerGlyph() {
	st := UseWithInit("spinner", func() *spinnerState { return &spinnerState{last: time.Now()} })
	if time.Since(st.last) >= 150*time.Millisecond {
		st.idx = (st.idx + 1) % len(spinnerFrames)
		st.last = time.Now()
	}
	Label(spinnerFrames[st.idx], FontSize(13), TextColorVec(txtMain))
	RequestNextFrame()
}

// focusableToggle renders widgets.ToggleSwitchExt with a label. Since
// shirei v0.6.10 the switch itself joins the tab ring, takes focus on
// click, flips on completed pointer clicks AND on Space/Enter (press→
// release), and paints its own focus ring — the wrapper is layout only, so
// it must not be focusable (double stop) and must not consume keys (double
// flip).
func focusableToggle(on *bool, label string) {
	Container(Attrs(PointerHand, Row, CrossMid, Gap(sp8), Corners(6)), func() {
		widgets.ToggleSwitchExt(on, widgets.ToggleSwitchAttrs{})
		Label(label, FontSize(13), TextColorVec(txtMain))
	})
}

// searchInput is the themed library filter field. Disabled while the
// library is empty. Its container id is captured so `/` can focus it from
// anywhere in the window.
func (m *model) searchInput() {
	if m.libraryEmpty() {
		Container(Attrs(Corners(radiusM), BackgroundVec(bgRaised), BorderWidth(1), BorderColorVec(border), Pad2(sp4, sp12), Grow(1), MinSize(140, fieldH), MaxSizeVec(Vec2{420, fieldH}), Clip, Trans(0.4)), func() {
			Container(Attrs(Row, Gap(sp8), CrossMid), func() {
				widgets.Icon(widgets.SymSearch, FontSize(13), TextColorVec(txtMuted))
				Label("Search…", FontSize(13), TextColorVec(txtMuted))
			})
		})
		return
	}
	themedInput(&m.filter, "Search…", widgets.SymSearch, Grow(1), MinSize(140, fieldH), MaxSizeVec(Vec2{420, fieldH}))
	m.searchID = GetLastId()
}

// fieldH is the shared text-field height: tight, one line of text plus a
// couple of pixels of breathing room.
const fieldH = 24

// editState is one themedInput's editing state: the caret position (a rune
// index into the buffer), the selection anchor (-1 when nothing is
// selected), and the blink clock. It persists per widget via UseWithInit.
type editState struct {
	cursor   int
	anchor   int
	blink    time.Time
	phase    bool
	dragging bool
	textRect Rect // screen rect of the text area, recorded each frame
	boxRect  Rect // screen rect of the field box, recorded each frame (width-stability seam)
	// Test seams, recorded each frame by the caret/text rendering:
	caretVisible bool    // the caret bar was painted this frame
	caretX       float32 // screen x of the caret bar's center
	inkRight     float32 // screen x of the text/hint ink's right edge
}

// selRange normalizes anchor/cursor into (lo, hi, hasSelection).
func (st *editState) selRange(bufLen int) (int, int, bool) {
	if st.cursor > bufLen {
		st.cursor = bufLen
	}
	if st.anchor < 0 || st.anchor == st.cursor {
		return st.cursor, st.cursor, false
	}
	if st.anchor > bufLen {
		st.anchor = bufLen
	}
	lo, hi := st.anchor, st.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi, true
}

// editKeys applies one frame of editing input to the buffer: cursor motion
// (arrows/Home/End), selection extension (Shift), select-all, copy/cut/
// paste (Ctrl+A/C/X/V via RequestTextCopy/RequestPaste), insert-replacing-
// selection, and range deletion. Keys are consumed so nothing leaks out.
func editKeys(buf *string, st *editState) {
	r := []rune(*buf)
	shift := GetInputState().Modifiers&ModShift != 0
	ctrl := GetInputState().Modifiers&ModCtrl != 0
	move := func(to int, extend bool) {
		if to < 0 {
			to = 0
		}
		if to > len(r) {
			to = len(r)
		}
		if extend {
			if st.anchor < 0 {
				st.anchor = st.cursor
			}
		} else {
			st.anchor = -1
		}
		st.cursor = to
		// Wake the blink: the caret is visible right after a move instead
		// of surfacing at the next phase flip (issue 9).
		st.phase = true
		st.blink = time.Now()
	}
	insert := func(s string) {
		rs := []rune(s)
		lo, hi, has := st.selRange(len(r))
		if has {
			out := make([]rune, 0, len(r)-hi+lo+len(rs))
			out = append(out, r[:lo]...)
			out = append(out, rs...)
			out = append(out, r[hi:]...)
			r = out
			st.cursor = lo + len(rs)
			st.anchor = -1
		} else {
			out := make([]rune, 0, len(r)+len(rs))
			out = append(out, r[:st.cursor]...)
			out = append(out, rs...)
			out = append(out, r[st.cursor:]...)
			r = out
			st.cursor += len(rs)
		}
		*buf = string(r)
	}
	deleteRange := func(lo, hi int) {
		out := make([]rune, 0, len(r)-(hi-lo))
		out = append(out, r[:lo]...)
		out = append(out, r[hi:]...)
		r = out
		st.cursor = lo
		st.anchor = -1
		*buf = string(r)
	}
	selText := func() string {
		lo, hi, has := st.selRange(len(r))
		if !has {
			return ""
		}
		return string(r[lo:hi])
	}

	if GetFrameInput().Text != "" {
		insert(GetFrameInput().Text)
		GetFrameInput().Text = ""
		st.phase = true
		st.blink = time.Now()
	}
	key := GetFrameInput().Key
	switch {
	case ctrl && key == KeyA:
		st.anchor = 0
		st.cursor = len(r)
	case ctrl && (key == KeyC || key == KeyX):
		if s := selText(); s != "" {
			RequestTextCopy(s)
			if key == KeyX {
				lo, hi, _ := st.selRange(len(r))
				deleteRange(lo, hi)
			}
		}
	case ctrl && key == KeyV:
		RequestPaste()
	case key == KeyLeft:
		move(st.cursor-1, shift)
	case key == KeyRight:
		move(st.cursor+1, shift)
	case key == KeyHome:
		move(0, shift)
	case key == KeyEnd:
		move(len(r), shift)
	case key == KeyDeleteBackward:
		if lo, hi, has := st.selRange(len(r)); has {
			deleteRange(lo, hi)
		} else if st.cursor > 0 {
			deleteRange(st.cursor-1, st.cursor)
		}
	case key == KeyDeleteForward:
		if lo, hi, has := st.selRange(len(r)); has {
			deleteRange(lo, hi)
		} else if st.cursor < len(r) {
			deleteRange(st.cursor, st.cursor+1)
		}
	case key == KeyEscape:
		*buf = ""
		st.cursor = 0
		st.anchor = -1
		Blur()
	case key == KeyEnter:
		// consumed: Enter must never leak to global handlers
	default:
		return
	}
	GetFrameInput().Key = KeyCodeNone
}

// shapedGlyphs shapes text exactly as the field's Label does (FontSize 13,
// default family) and returns the flattened glyph run.
func shapedGlyphs(text string) []Glyph {
	return shapedGlyphsAt(text, 13)
}

// shapedGlyphsAt shapes text at the given font size (default family) and
// returns the flattened glyph run.
func shapedGlyphsAt(text string, size float32) []Glyph {
	var ta TextStyleAttrs
	FontSize(size)(&ta)
	shaped := ShapeText(text, ta)
	var gs []Glyph
	for _, line := range shaped.Lines {
		for _, seg := range line.Segments {
			gs = append(gs, seg.Glyphs...)
		}
	}
	return gs
}

// textWidth is the shaped advance width of text in pixels.
func textWidth(text string) float32 {
	return textWidthAt(text, 13)
}

// textWidthAt is the shaped advance width of text in pixels at the given
// font size.
func textWidthAt(text string, size float32) float32 {
	w := float32(0)
	for _, g := range shapedGlyphsAt(text, size) {
		w += g.XAdvance
	}
	return w
}

// hitIndex maps an x offset inside the text area to a rune caret index:
// each glyph owns the span up to its midpoint (click left of the midpoint
// lands before it, right lands after). Glyph clusters are rune indices.
func hitIndex(text string, relX float32) int {
	r := []rune(text)
	if relX <= 0 || len(r) == 0 {
		return 0
	}
	acc := float32(0)
	for _, g := range shapedGlyphs(text) {
		if relX < acc+g.XAdvance/2 {
			return int(g.Cluster)
		}
		acc += g.XAdvance
	}
	return len(r)
}

// wordRange returns the rune range of the word around idx (letters, digits,
// underscores); a non-word rune yields just itself.
func wordRange(r []rune, idx int) (int, int) {
	if len(r) == 0 {
		return 0, 0
	}
	if idx >= len(r) {
		idx = len(r) - 1
	}
	word := func(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' }
	if !word(r[idx]) {
		return idx, idx + 1
	}
	lo, hi := idx, idx+1
	for lo > 0 && word(r[lo-1]) {
		lo--
	}
	for hi < len(r) && word(r[hi]) {
		hi++
	}
	return lo, hi
}

// editMouse applies one frame of mouse editing: a click moves the caret to
// the clicked glyph, shift+click extends the selection, double-click selects
// the word, triple-click selects all, and dragging extends the selection.
// Runs whether or not the field is focused so the focusing click also
// positions the caret.
func editMouse(buf *string, st *editState) {
	r := []rune(*buf)
	relX := GetInputState().MousePoint[0] - st.textRect.Origin[0]
	wake := func() { st.phase, st.blink = true, time.Now() }
	if st.dragging {
		if GetFrameInput().Mouse == MouseRelease {
			st.dragging = false
			if st.anchor == st.cursor {
				st.anchor = -1
			}
		} else {
			st.cursor = hitIndex(*buf, relX)
			wake()
		}
		return
	}
	if GetFrameInput().Mouse != MouseClick || !IsHovered() {
		return
	}
	idx := hitIndex(*buf, relX)
	switch {
	case GetInputState().Modifiers&ModShift != 0:
		if st.anchor < 0 {
			st.anchor = st.cursor
		}
		st.cursor = idx
	case GetFrameInput().ClickCount >= 3:
		st.anchor, st.cursor = 0, len(r)
	case GetFrameInput().ClickCount == 2:
		st.anchor, st.cursor = wordRange(r, idx)
	default:
		st.cursor, st.anchor, st.dragging = idx, idx, true
	}
	wake()
}

// caretFloat paints the text caret as a direct float child of the text
// row: out of the layout flow, so caret motion and blink never reflow
// the text — the letters stay put (issue 9). x is the caret's center
// relative to the text row's origin. (The caret must float in the row
// itself, not in a zero-width anchor: a float nested in a container
// without in-flow children is never sized by shirei's layout and paints
// nothing.) The bar spans the row height and is NoAnimate — shirei eases
// relativeOrigin by default, and an easing caret would glide over the
// text. The blink clock advances here; the bar paints only while focused
// and the phase is on.
func caretFloat(st *editState, focused bool, x float32) {
	if !focused {
		st.caretVisible = false
		return
	}
	if time.Since(st.blink) > 500*time.Millisecond {
		st.phase = !st.phase
		st.blink = time.Now()
	}
	st.caretVisible = st.phase
	if st.phase {
		h := st.textRect.Size[1]
		if h == 0 {
			h = 16
		}
		Container(Attrs(Float(x-1, 0), FixSize(2, h), NoAnimate, BackgroundVec(focusBorder)), func() {})
	}
	RequestNextFrame()
}

// advanceUpTo returns the shaped advance of the runes before cursor —
// the caret's x offset inside the single shaped text run.
func advanceUpTo(text string, cursor int) float32 {
	w := float32(0)
	for _, g := range shapedGlyphs(text) {
		if int(g.Cluster) >= cursor {
			break
		}
		w += g.XAdvance
	}
	return w
}

// inkRightEdge returns the screen x of the right edge of the last
// rendered element (a text/hint Label), resolved from the previous
// frame — the letters-don't-move seam.
func inkRightEdge() float32 {
	r := GetScreenRectOf(GetLastId())
	return r.Origin[0] + r.Size[0]
}

// themedInput is themedInputState with the state kept internally; it is
// THE reusable text field — search, the version field, and the
// launch-template field all share it.
func themedInput(buf *string, hint string, icon widgets.IconGlyph, sizing ...AttrsFn) {
	themedInputState(buf, hint, icon, nil, sizing...)
}

// themedInputState is themedInput with a caller-owned edit state (tests
// drive the same editing flow and assert on st directly).
func themedInputState(buf *string, hint string, icon widgets.IconGlyph, st *editState, sizing ...AttrsFn) {
	// TextEntry keeps the default arrow cursor over the field (v0.17
	// hover-chain rule): the caret, not a hand, signals editability.
	box := Attrs(Focusable, TextEntry, Row, CrossMid, Corners(radiusM), BackgroundVec(bgRaised), BorderWidth(1), BorderColorVec(border), Pad2(2, sp12), Clip)
	Container(AttrsWith(box, sizing...), func() {
		st0 := st
		if st0 != nil {
			st0.boxRect = GetScreenRectOf(CurrentId())
		}
		if st == nil {
			st = UseWithInit("edit:"+hint, func() *editState {
				return &editState{cursor: len([]rune(*buf)), anchor: -1, blink: time.Now(), phase: true}
			})
		}
		FocusOnClick()
		editMouse(buf, st)
		// HasFocus() only reports the container currently being built —
		// capture it here, at the box (the focused element), not inside the
		// row below (where it would always read false and the caret never
		// rendered).
		focused := HasFocus()
		if focused {
			ModAttrs(func(a *AttrSet) { a.BorderColor = focusBorder })
			editKeys(buf, st)
		}
		r := []rune(*buf)
		lo, hi, hasSel := st.selRange(len(r))
		Container(Attrs(Row, Gap(sp8), CrossMid, Grow(1)), func() {
			if icon != widgets.NoIcon {
				widgets.Icon(icon, FontSize(13), TextColorVec(txtMuted))
			}
			Container(Attrs(Row, Gap(0), CrossMid, Grow(1), MinSize(0, 16)), func() {
				st.textRect = GetScreenRect()
				switch {
				case hasSel:
					// The selection highlight needs the split (its own
					// background box); the caret floats at the selection
					// boundary, out of the layout flow like everywhere
					// else.
					Label(string(r[:lo]), FontSize(13), TextColorVec(txtMain))
					Container(Attrs(Row, Gap(0), CrossMid, BackgroundVec(selBg), Corners(2)), func() {
						Label(string(r[lo:hi]), FontSize(13), TextColorVec(txtMain))
					})
					Label(string(r[hi:]), FontSize(13), TextColorVec(txtMain))
					st.inkRight = inkRightEdge()
					x := advanceUpTo(*buf, st.cursor)
					st.caretX = st.textRect.Origin[0] + x
					caretFloat(st, focused, x)
				case len(r) > 0:
					// ONE Label for the whole buffer: shaped once, so the
					// letters sit at identical positions for every caret
					// position and blink phase (issue 9).
					Label(*buf, FontSize(13), TextColorVec(txtMain))
					st.inkRight = inkRightEdge()
					x := advanceUpTo(*buf, st.cursor)
					st.caretX = st.textRect.Origin[0] + x
					caretFloat(st, focused, x)
				default:
					// Empty: the hint stays even when focused (issue 8) and
					// the caret floats at position 0 over the hint's left
					// edge — focused empty fields no longer look dead
					// (issue 9), and the floating bar takes no layout
					// space, so the field still never resizes.
					Label(hint, FontSize(13), TextColorVec(txtMuted))
					st.inkRight = inkRightEdge()
					st.caretX = st.textRect.Origin[0]
					caretFloat(st, focused, 0)
				}
			})
		})
	})
}

// viewSwitch is the grid/list segmented toggle with icon segments; disabled
// while the library is empty. The OUTER wrapper is the single Tab stop for
// the binary choice (Focusable + CycleFocusOnTab + FocusOnClick, mirroring
// the sortDropdown trigger): Tab reaches it with a focus ring (BorderWidth
// stays at 1 — only the color flips, so the ring never shifts layout), and
// Enter/Space — consumed so nothing downstream re-triggers — toggle the
// view through the session, honoring the same disabled guard as the
// segment PressAction. The segments themselves stay unfocusable.
func (m *model) viewSwitch() {
	disabled := m.libraryEmpty()
	// Per-frame seam reset, mirroring the sortDropdown's sortTriggerID /
	// sortFocusRing discipline: the seams describe the frame being built.
	m.viewSwitchID = nil
	m.viewSwitchFocusRing = false
	Container(Attrs(Focusable, PointerHand, Row, Corners(radiusM), Clip, BorderWidth(1), BorderColorVec(border)), func() {
		FocusOnClick()
		m.viewSwitchID = CurrentId()
		if HasFocus() {
			m.viewSwitchFocusRing = true
			ModAttrs(func(a *AttrSet) { a.BorderColor = focusBorder })
			if GetFrameInput().Key == KeyEnter || GetFrameInput().Key == KeySpace {
				GetFrameInput().Key = KeyCodeNone
				if !disabled && m.sess != nil {
					m.sess.ToggleView()
				}
			}
		}
		m.viewSegment(widgets.SymGrid, "Grid", ui.ViewGrid, disabled)
		m.viewSegment(widgets.SymList, "List", ui.ViewList, disabled)
	})
}

// viewSegment is one half of the view switch; activating it flips the view
// mode through the session.
func (m *model) viewSegment(icon widgets.IconGlyph, label string, mode ui.ViewMode, disabled bool) {
	selected := m.state.Mode == mode
	fg := txtMuted
	if selected {
		fg = txtMain
	}
	Container(Attrs(PointerHand, Row, CrossMid, Gap(sp4), Pad2(sp4, sp8)), func() {
		if mode == ui.ViewList {
			m.listSegRect = GetScreenRectOf(CurrentId())
		}
		switch {
		case disabled:
			ModAttrs(Trans(0.35))
		case selected:
			ModAttrs(BackgroundVec(accent))
		case IsHovered():
			ModAttrs(BackgroundVec(bgRaised))
		}
		widgets.Icon(icon, FontSize(13), TextColorVec(fg))
		Label(label, FontSize(12), TextColorVec(fg))
		if !disabled && PressAction() && m.sess != nil && !selected {
			m.sess.ToggleView()
		}
	})
}

// shortenPath ellipsizes a long path at the front so the distinctive tail
// (the directory name) stays visible.
func shortenPath(p string, max int) string {
	r := []rune(p)
	if len(r) <= max {
		return p
	}
	return "…" + string(r[len(r)-max+1:])
}

// optiBadge is the OptiScaler pill for a row, named after the installed
// distribution: "<ForkName> <version>" for a fork, "OptiScaler <version>"
// for upstream (its distribution name IS OptiScaler). Blue and
// external-marked for unmanaged on-disk installs. ok=false for rows
// without an OptiScaler install — those render no pill and no version
// dropdown.
func optiBadge(e *ui.GameRow) (ui.Badge, bool) {
	external := e.Status == domain.StatusExternal
	name := e.ForkLabel()
	if name == "" {
		name = "OptiScaler"
	}
	switch {
	case e.OptiScalerVersion != "" && external:
		return ui.Badge{Label: "✦ " + name + " " + e.OptiScalerVersion + " · external", Tone: ui.ToneBlue}, true
	case e.OptiScalerVersion != "":
		return ui.Badge{Label: "✦ " + name + " " + e.OptiScalerVersion, Tone: ui.TonePurple}, true
	case external:
		return ui.Badge{Label: "✦ " + name + " · external", Tone: ui.ToneBlue}, true
	case e.Status == domain.StatusCommitted:
		return ui.Badge{Label: "✦ " + name, Tone: ui.TonePurple}, true
	}
	return ui.Badge{}, false
}

// versionPills is the install-version badge set for a row: the OptiScaler
// pill (versioned when the installed version is known, blue and
// external-marked for unmanaged on-disk installs), one pill per component
// version, and a Proton marker for prefixed games.
func versionPills(e *ui.GameRow) []ui.Badge {
	var out []ui.Badge
	if b, ok := optiBadge(e); ok {
		out = append(out, b)
	}
	for _, c := range e.Components {
		out = append(out, ui.Badge{Label: c, Tone: componentTone(c)})
	}
	if e.CompatPrefix != "" {
		out = append(out, ui.Badge{Label: "Proton", Tone: ui.ToneBlue})
	}
	return out
}

// componentTone colors a versioned component pill like its tech badge.
func componentTone(label string) ui.Tone {
	switch {
	case strings.HasPrefix(label, "DLSS"):
		return ui.ToneGreen
	case strings.HasPrefix(label, "FSR"):
		return ui.ToneRed
	case strings.HasPrefix(label, "XeSS"):
		return ui.ToneBlue
	default:
		return ui.ToneGray
	}
}

// tierPillStyle maps a ProtonDB tier to its pill background: precious-metal
// tiers get their metal's hue, borked is red, pending is muted. ok=false
// for empty or unknown tiers (no pill rendered).
func tierPillStyle(tier string) (bg Vec4, ok bool) {
	switch tier {
	case "platinum":
		return Vec4{210, 60, 42, 1}, true
	case "gold":
		return Vec4{45, 70, 45, 1}, true
	case "silver":
		return Vec4{220, 8, 55, 1}, true
	case "bronze":
		return Vec4{25, 65, 45, 1}, true
	case "borked":
		return Vec4{5, 60, 40, 1}, true
	case "pending":
		return Vec4{220, 10, 35, 1}, true
	}
	return Vec4{}, false
}

// protonTierPill renders the ProtonDB tier badge; a no-op for empty or
// unknown tiers.
func (m *model) protonTierPill(tier string) {
	bg, ok := tierPillStyle(tier)
	if !ok {
		return
	}
	Container(Attrs(Pad2(3, 6), Corners(radiusS), BackgroundVec(bg)), func() {
		m.tierPillRect = GetScreenRectOf(CurrentId())
		Label(tier, TextColor(0, 0, 96, 1), FontSize(11))
	})
}

// launchable reports whether a row carries enough identity to launch:
// store games go by AppID, manual/GOG games by executable path.
func launchable(e *ui.GameRow) bool {
	return e.AppID != "" || e.ExePath != ""
}

// dropdownState is one version dropdown's per-container state (a shirei
// Use[T] hook, mirroring widgets/menu.go's MenuState): the open flag plus
// the trigger and popup container ids the click-outside check needs. hl is
// the keyboard-highlight row index; -1 means "re-initialize on the next
// popup frame" (set on every open, so the highlight starts on the current
// item and never leaks across opens). prevMouse is the mouse position seen
// by the previous popup frame: hover adopts the highlight only when the
// mouse actually MOVED, so a mouse resting over a row cannot overwrite the
// trigger's arrow-key navigation.
// versionDDItem is the dropdown's observability seam: one entry per
// rendered popup row (version, current-tick, screen rect for click tests,
// keyboard highlight). label is the rendered row text as the user reads
// it (== version for concrete version rows) — the seam tests assert on
// what the menu actually offers.
type versionDDItem struct {
	version string
	label   string
	ticked  bool
	rect    Rect
	hl      bool
}

// versionMenuRow is one rendered version-dropdown row: dispatch is the
// value handed to the session on pick (the literal "latest" for the Latest
// row, which the switch re-resolves at pick time), guard is the concrete
// tag the tick and the same-version no-op compare against (the Latest row
// guards with the startup-resolved tag, so picking it while already at the
// latest stays a no-op), and label is the text the user reads.
type versionMenuRow struct {
	dispatch string
	guard    string
	label    string
}

// versionMenuRows composes the dropdown's rows: Session.Versions(dir) with
// the startup-resolved latest rendered as a first-class "Latest (…)"
// option. The option ABSORBS the concrete entry when that entry IS the
// latest (one row, one tick, no duplicate) or PREPENDS when the latest is
// absent from the list (it is the maximum, so it sorts first). No known
// latest (offline boot) = the plain concrete list.
func versionMenuRows(m *model, dir string) []versionMenuRow {
	versions := m.sess.Versions(dir)
	latest := m.sess.LatestKnown()
	rows := make([]versionMenuRow, 0, len(versions)+1)
	absorbed := false
	for _, v := range versions {
		if latest != "" && !absorbed && version.Compare(v, latest) == 0 {
			rows = append(rows, versionMenuRow{dispatch: "latest", guard: latest, label: "Latest (" + latest + ")"})
			absorbed = true
			continue
		}
		rows = append(rows, versionMenuRow{dispatch: v, guard: v, label: v})
	}
	if latest != "" && !absorbed {
		rows = append([]versionMenuRow{{dispatch: "latest", guard: latest, label: "Latest (" + latest + ")"}}, rows...)
	}
	return rows
}

// versionDropdown replaces the static OptiScaler version pill with a
// per-game version selector: a pill-sized trigger labeled with the badge
// text (current version) and a sorted-down arrow, opening a dark popup of
// Session.Versions(dir) — installed ∪ cached ∪ preference, semver-desc —
// with the current version ticked. Picking a DIFFERENT version dispatches
// Session.SwitchVersion via dispatchSwitchVersion; re-picking the current
// one is a deliberate no-op (S13): the widget does not even dispatch.
//
// I/O DISCIPLINE: the CLOSED trigger renders only the row's current
// version (zero I/O — Versions walks the bundle cache with os.ReadDir,
// which per card per frame would be pathological); the list is computed on
// the frame the dropdown OPENS and on each frame while it stays open, so a
// bundle cached mid-session appears on the next open.
//
// ONE OPEN AT A TIME: the open flag itself is per-container (the Use hook
// above), but m.openDropdownDir names the single open dropdown; a dropdown
// that finds another dir owning the field closes itself, so cards
// re-rendering every frame can never leave a stale popup behind.
//
// The popup renders through Popup (root scope) precisely so it escapes the
// card's Clip, and floats below the trigger clamped to the window — the
// local modal()/menu.go anchoring pattern, NOT upstream MenuButtonExt,
// whose _menuBG surface is theme-locked light.
func (m *model) versionDropdown(e *ui.GameRow, label string, tone ui.Tone) {
	// Without a session there is nothing to list or dispatch: fall back to
	// the static pill (the same sess == nil gating the card buttons use).
	if m.sess == nil {
		badgePill(label, tone)
		return
	}
	st := Use[dropdownState]("version-dropdown")
	if st.open && m.openDropdownDir != e.InstallDir {
		st.open = false
	}
	// A closed dropdown clears the shared item list only when NO dropdown is
	// open for this game — card and panel each render one, and the closed one
	// must not stomp the open one's items.
	if !st.open && m.openDropdownDir != e.InstallDir && m.versionDDItemsFor == e.InstallDir {
		m.versionDDItems = nil
		m.versionDDItemsFor = ""
	}
	// Trigger: badgePill geometry (Pad2(3, 6), FontSize 11) so the pill row
	// height — and with it cardContentH — is untouched.
	enterPick := false
	Container(Attrs(Focusable, PointerHand, Row, CrossMid, Gap(sp4), Pad2(3, 6), Corners(radiusS), BackgroundVec(toneColor(tone))), func() {
		FocusOnClick()
		m.ddTriggerID = CurrentId()
		if m.versionDDRects == nil {
			m.versionDDRects = map[string]Rect{}
		}
		m.versionDDRects[e.InstallDir] = GetScreenRectOf(CurrentId())
		st.btnID = CurrentId()
		activated := false
		if HasFocus() {
			// Inside a card the card HOSTS the contextual ring (focus is
			// within its subtree) and the pill adds nothing — one ring per
			// context. Outside a card (detail panel) the pill wears its
			// own.
			if m.cardRingOnDir != e.InstallDir {
				m.ddFocusRing = true
				ModAttrs(func(a *AttrSet) {
					a.BorderWidth = 2
					a.BorderColor = focusBorder
				})
			}
			if st.open {
				// With the popup open the trigger owns menu navigation via
				// the shared dropdown keys (Up/Down highlight, Enter pick,
				// Space closes); all consumed so no frame-end fallback can
				// also see them.
				var spaceToggle bool
				enterPick, spaceToggle = dropdownMenuKeys(st, len(m.versionDDItems))
				if spaceToggle {
					activated = true
				}
			} else if GetFrameInput().Key == KeyEnter || GetFrameInput().Key == KeySpace {
				GetFrameInput().Key = KeyCodeNone
				activated = true
			}
		}
		Label(label, FontSize(11), TextColor(0, 0, 96, 1))
		dropdownArrow()
		if PressAction() {
			activated = true
		}
		if activated {
			st.open = !st.open
			if st.open {
				m.openDropdownDir = e.InstallDir
				st.hl = -1 // re-initialize the highlight on the popup frame
			} else if m.openDropdownDir == e.InstallDir {
				m.openDropdownDir = ""
			}
		}
	})
	if st.open {
		dir := e.InstallDir
		current := e.OptiScalerVersion
		dropdownPopup(st, 0, 360, func(mouseMoved bool) {
			// Computed here, never on closed frames: Versions walks the
			// bundle cache (see the I/O note above).
			rows := versionMenuRows(m, dir)
			if st.hl < 0 {
				// Open-time init: the highlight starts on the ticked
				// (current version) row, 0 when nothing is ticked.
				st.hl = 0
				for i, r := range rows {
					if version.Compare(r.guard, current) == 0 {
						st.hl = i
						break
					}
				}
			}
			m.versionDDItems = m.versionDDItems[:0]
			m.versionDDItemsFor = dir
			for i, r := range rows {
				r := r
				ticked := version.Compare(r.guard, current) == 0
				dropdownRow(st, i, enterPick, mouseMoved,
					func(hl bool) {
						// Fixed-width tick column keeps version labels aligned.
						tick := " "
						if ticked {
							tick = "✓"
						}
						Label(tick, FontSize(12), TextColorVec(txtMain))
						Label(r.label, FontSize(12), TextColorVec(txtMain))
						m.versionDDItems = append(m.versionDDItems, versionDDItem{version: r.dispatch, label: r.label, ticked: ticked, rect: GetScreenRectOf(CurrentId()), hl: hl})
					},
					func(keyboard bool) {
						// Re-picking the current version is a deliberate
						// no-op (S13): the widget does not even dispatch.
						// The guard is the row's CONCRETE key (the
						// resolved tag for the Latest row), never its
						// dispatch literal.
						if version.Compare(r.guard, current) != 0 {
							m.dispatchSwitchVersion(dir, r.dispatch)
						}
						m.openDropdownDir = ""
						if keyboard {
							// Enter on the trigger picks the highlighted
							// row; focus never left it, but re-assert it
							// against any settle-frame blur.
							FocusImmediateOn(st.btnID)
						}
					})
			}
		})
	}
	// Dismissal, AFTER the popup rendered so clicks inside it still register
	// (menu.go:84's ordering): Esc closes without dispatch and is consumed
	// so the global Esc handler cannot also close the detail panel; a click
	// outside both trigger and popup closes without dispatch.
	dropdownDismiss(st, func() { m.openDropdownDir = "" })
}

// dropdownPos anchors the popup below the trigger, clamped to the window —
// a local copy of widgets/menu.go's unexported _getPositionRelativeTo.
func dropdownPos(anchorID ContainerId) Vec2 {
	targetRect := GetResolvedRectOf(anchorID)
	const sp = 4
	pos := targetRect.Origin
	pos[1] += targetRect.Size[1] + sp
	selfSize := GetResolvedSize()
	if pos[0]+selfSize[0] > GetHost().WindowSize[0] {
		pos[0] = GetHost().WindowSize[0] - selfSize[0] - sp
	}
	if pos[1]+selfSize[1] > GetHost().WindowSize[1] {
		pos[1] = GetHost().WindowSize[1] - selfSize[1] - sp
	}
	pos[0] = max(0, pos[0])
	pos[1] = max(0, pos[1])
	return pos
}

// sortMenuItem is the sort dropdown's observability seam: one entry per
// rendered popup row (label, screen rect for click tests, keyboard
// highlight), mirroring versionDDItem.
type sortMenuItem struct {
	label string
	rect  Rect
	hl    bool
}

// sortDropdown replaces the toolbar's upstream MenuButtonExt sort control —
// unfocusable, keyboard-unreachable, and theme-locked to a light popup via
// widgets._menuBG — with a local trigger and a dark popup, modeled
// line-for-line on versionDropdown.
//
// Since shirei v0.6.10 the widgets.ButtonExt face is itself the tab stop,
// focus owner, and click source (pointer release, or Space/Enter press→
// release), so the wrapper is geometry + focus ring only. With the popup
// open the app owns menu navigation; those keys are consumed BEFORE the
// face renders — left alone, the face would arm Enter/Space and toggle the
// popup closed on their release.
//
// The popup renders through Popup (root scope) with the dark panel tokens and
// floats below the trigger via dropdownPos. The items are the two sort modes
// with the same icons, labels, and setSort calls the old MenuItems used —
// composed locally instead of via widgets.MenuItem because MenuItem paints
// its row with the theme-locked light _menuBG.
func (m *model) sortDropdown() {
	st := Use[dropdownState]("sort-dropdown")
	disabled := m.libraryEmpty()
	if st.open && disabled {
		// The trigger is inert while the library is empty, so an open
		// dropdown makes no sense: a state left open across the empty
		// transition (or inherited via a hook slot retained on the
		// process-wide identity tree) closes instead of floating over
		// a trigger that can no longer own it.
		st.open = false
	}
	// Per-frame seam reset, mirroring gameCard's ddTriggerID/ddFocusRing
	// discipline: the seams describe the frame being built, so a closed
	// dropdown exposes no items. lastTriggerID keeps the face id from LAST
	// frame for the pre-consume focus check below.
	lastTriggerID := m.sortTriggerID
	m.sortTriggerID = nil
	m.sortFocusRing = false
	if !st.open {
		m.sortMenuItems = nil
	}
	enterPick := false
	activated := false
	Container(Attrs(PointerHand, Corners(6)), func() {
		// The face (ButtonExt's container) is the focus owner from the
		// previous frame. Attrs must change BEFORE children (shirei panics
		// otherwise), so both the ring and the nav-key consumption read the
		// face's focus state here, before ButtonExt renders.
		faceFocused := lastTriggerID != nil && IdHasFocus(lastTriggerID)
		if faceFocused {
			m.sortFocusRing = true
			ModAttrs(func(a *AttrSet) {
				a.BorderWidth = 2
				a.BorderColor = focusBorder
			})
		}
		if st.open && faceFocused {
			// With the popup open the trigger owns menu navigation via the
			// shared dropdown keys (Up/Down highlight, Enter pick, Space
			// closes); all consumed so the face cannot also see them.
			var spaceToggle bool
			enterPick, spaceToggle = dropdownMenuKeys(st, len(m.sortMenuItems))
			if spaceToggle && !disabled {
				activated = true
			}
		}
		if widgets.ButtonExt("Sort: "+sortLabel(m.state.Sort), widgets.ButtonAttrs{Icon: widgets.TypArrowSortedDown, Disabled: disabled}, widgets.DefaultButtonLook()) {
			activated = true
		}
		// The face is the nav-key seam, the popup anchor, and the ring's
		// state source for the next frame.
		m.sortTriggerID = GetLastId()
		st.btnID = m.sortTriggerID
		if activated {
			st.open = !st.open
			if st.open {
				st.hl = -1 // re-initialize the highlight on the popup frame
			}
		}
	})
	if st.open {
		dropdownPopup(st, 0, 0, func(mouseMoved bool) {
			if st.hl < 0 {
				// Open-time init: the highlight starts on the current
				// sort mode's row.
				st.hl = 0
				if m.state.Sort == ui.SortName {
					st.hl = 1
				}
			}
			m.sortMenuItems = m.sortMenuItems[:0]
			m.sortItem(st, widgets.SymStar, "Default (actionable first)", ui.SortDefault, enterPick, mouseMoved)
			m.sortItem(st, widgets.NoIcon, "Name (A–Z)", ui.SortName, enterPick, mouseMoved)
		})
	}
	// Dismissal, AFTER the popup rendered so clicks inside it still register
	// (versionDropdown's ordering): Esc closes without dispatch and is
	// consumed here — the toolbar renders before handleGlobalKeys, so the
	// global Esc handler cannot also close the detail panel; a click outside
	// both trigger and popup closes without dispatch.
	dropdownDismiss(st, nil)
}

// sortItem is one row of the sort dropdown's popup: recorded in the
// sortMenuItems seam, calling setSort and closing on a pick. The
// highlighted row (keyboard or hover) paints the hover accent; enterPick
// is the trigger's "Enter was pressed while open" signal and activates the
// highlighted row exactly like a click. hoverAdopt is the popup frame's
// mouse-motion gate: hover adopts the highlight only on actual mouse
// motion, so a resting mouse cannot overwrite an arrow-key move.
func (m *model) sortItem(st *dropdownState, icon widgets.IconGlyph, label string, mode ui.SortMode, enterPick, hoverAdopt bool) {
	dropdownRow(st, len(m.sortMenuItems), enterPick, hoverAdopt,
		func(hl bool) {
			if icon != widgets.NoIcon {
				widgets.Icon(icon, FontSize(12), TextColorVec(txtMain))
			}
			Label(label, FontSize(12), TextColorVec(txtMain))
			m.sortMenuItems = append(m.sortMenuItems, sortMenuItem{label: label, rect: GetScreenRectOf(CurrentId()), hl: hl})
		},
		func(bool) {
			m.setSort(mode)
			// A pick is a click outside the trigger, so FocusOnClick blurred
			// it on the down frame of this gesture; hand focus back. The
			// toolbar renders at a stable path every frame (above the
			// conditional detail-panel Row), so the id resolves immediately —
			// no deferred re-assert (unlike actionList's listFocusPending).
			FocusImmediateOn(st.btnID)
		})
}
