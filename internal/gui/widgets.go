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

// focusableButton renders widgets.Button inside a Focusable wrapper so the
// global Tab / Shift-Tab focus cycle reaches it (shirei's Button itself is
// not focusable). Enter or Space while focused activates the button, and the
// key is consumed so no later widget in the frame can double-fire.
func focusableButton(icon rune, label string) bool {
	return focusableButtonExt(label, widgets.ButtonAttrs{Icon: icon})
}

// focusableButtonExt is focusableButton with full ButtonAttrs control
// (disabled state, accent, sizing).
func focusableButtonExt(label string, attrs widgets.ButtonAttrs) bool {
	var activated bool
	Container(Attrs(Focusable, Corners(6)), func() {
		CycleFocusOnTab()
		if HasFocus() {
			ModAttrs(func(a *AttrSet) {
				a.BorderWidth = 2
				a.BorderColor = focusBorder
			})
			if FrameInput.Key == KeyEnter || FrameInput.Key == KeySpace {
				FrameInput.Key = KeyCodeNone
				activated = true
			}
		}
		if widgets.ButtonExt(label, attrs) {
			activated = true
		}
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

// focusableToggle renders widgets.ToggleSwitchExt inside a Focusable row so
// the Tab cycle reaches it (the switch itself is not focusable): Enter or
// Space while focused flips the bound value, and the key is consumed. Mouse
// clicks flip via the switch itself.
func focusableToggle(on *bool, label string) {
	Container(Attrs(Focusable, Row, CrossMid, Gap(sp8), Corners(6)), func() {
		CycleFocusOnTab()
		if HasFocus() {
			ModAttrs(func(a *AttrSet) {
				a.BorderWidth = 2
				a.BorderColor = focusBorder
			})
			if FrameInput.Key == KeyEnter || FrameInput.Key == KeySpace {
				FrameInput.Key = KeyCodeNone
				*on = !*on
			}
		}
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
	shift := InputState.Modifiers&ModShift != 0
	ctrl := InputState.Modifiers&ModCtrl != 0
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

	if FrameInput.Text != "" {
		insert(FrameInput.Text)
		FrameInput.Text = ""
		st.phase = true
		st.blink = time.Now()
	}
	key := FrameInput.Key
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
	FrameInput.Key = KeyCodeNone
}

// shapedGlyphs shapes text exactly as the field's Label does (FontSize 13,
// default family) and returns the flattened glyph run.
func shapedGlyphs(text string) []Glyph {
	var ta TextAttrSet
	FontSize(13)(&ta)
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
	w := float32(0)
	for _, g := range shapedGlyphs(text) {
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
	relX := InputState.MousePoint[0] - st.textRect.Origin[0]
	wake := func() { st.phase, st.blink = true, time.Now() }
	if st.dragging {
		if FrameInput.Mouse == MouseRelease {
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
	if FrameInput.Mouse != MouseClick || !IsHovered() {
		return
	}
	idx := hitIndex(*buf, relX)
	switch {
	case InputState.Modifiers&ModShift != 0:
		if st.anchor < 0 {
			st.anchor = st.cursor
		}
		st.cursor = idx
	case FrameInput.ClickCount >= 3:
		st.anchor, st.cursor = 0, len(r)
	case FrameInput.ClickCount == 2:
		st.anchor, st.cursor = wordRange(r, idx)
	default:
		st.cursor, st.anchor, st.dragging = idx, idx, true
	}
	wake()
}

// caretBar renders the caret when the blink phase is on and advances the
// 500ms blink clock (real caret feel: flush with the text, blinking).
func caretBar(st *editState, focused bool) {
	if !focused {
		return
	}
	if time.Since(st.blink) > 500*time.Millisecond {
		st.phase = !st.phase
		st.blink = time.Now()
	}
	if st.phase {
		Container(Attrs(FixSize(2, 13), BackgroundVec(focusBorder)), func() {})
	}
	RequestNextFrame()
}

// themedInput is themedInputState with the state kept internally; it is
// THE reusable text field — search, the version field, and the
// launch-template field all share it.
func themedInput(buf *string, hint string, icon rune, sizing ...AttrsFn) {
	themedInputState(buf, hint, icon, nil, sizing...)
}

// themedInputState is themedInput with a caller-owned edit state (tests
// drive the same editing flow and assert on st directly).
func themedInputState(buf *string, hint string, icon rune, st *editState, sizing ...AttrsFn) {
	box := Attrs(Focusable, Row, CrossMid, Corners(radiusM), BackgroundVec(bgRaised), BorderWidth(1), BorderColorVec(border), Pad2(2, sp12), Clip)
	Container(AttrsWith(box, sizing...), func() {
		if st == nil {
			st = UseWithInit("edit:"+hint, func() *editState {
				return &editState{cursor: len([]rune(*buf)), anchor: -1, blink: time.Now(), phase: true}
			})
		}
		CycleFocusOnTab()
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
			if icon != 0 {
				widgets.Icon(icon, FontSize(13), TextColorVec(txtMuted))
			}
			Container(Attrs(Row, Gap(0), CrossMid, Grow(1)), func() {
				st.textRect = GetScreenRect()
				switch {
				case hasSel:
					Label(string(r[:lo]), FontSize(13), TextColorVec(txtMain))
					Container(Attrs(Row, Gap(0), CrossMid, BackgroundVec(selBg), Corners(2)), func() {
						Label(string(r[lo:hi]), FontSize(13), TextColorVec(txtMain))
						if st.cursor == hi {
							caretBar(st, focused)
						}
					})
					if st.cursor == lo {
						caretBar(st, focused)
					}
					Label(string(r[hi:]), FontSize(13), TextColorVec(txtMain))
				case len(r) > 0:
					Label(string(r[:st.cursor]), FontSize(13), TextColorVec(txtMain))
					caretBar(st, focused)
					Label(string(r[st.cursor:]), FontSize(13), TextColorVec(txtMain))
				case focused:
					caretBar(st, focused)
				default:
					Label(hint, FontSize(13), TextColorVec(txtMuted))
				}
			})
		})
	})
}

// viewSwitch is the grid/list segmented toggle with icon segments; disabled
// while the library is empty.
func (m *model) viewSwitch() {
	disabled := m.libraryEmpty()
	Container(Attrs(Row, Corners(radiusM), Clip, BorderWidth(1), BorderColorVec(border)), func() {
		m.viewSegment(widgets.SymGrid, "Grid", ui.ViewGrid, disabled)
		m.viewSegment(widgets.SymList, "List", ui.ViewList, disabled)
	})
}

// viewSegment is one half of the view switch; activating it flips the view
// mode through the session.
func (m *model) viewSegment(icon rune, label string, mode ui.ViewMode, disabled bool) {
	selected := m.state.Mode == mode
	fg := txtMuted
	if selected {
		fg = txtMain
	}
	Container(Attrs(Row, CrossMid, Gap(sp4), Pad2(sp4, sp8)), func() {
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

// optiBadge is the OptiScaler pill for a row: versioned when the installed
// version is known, blue and external-marked for unmanaged on-disk
// installs. ok=false for rows without an OptiScaler install — those render
// no pill and no version dropdown.
func optiBadge(e *ui.GameRow) (ui.Badge, bool) {
	external := e.Status == domain.StatusExternal
	switch {
	case e.OptiScalerVersion != "" && external:
		return ui.Badge{Label: "✦ OptiScaler " + e.OptiScalerVersion + " · external", Tone: ui.ToneBlue}, true
	case e.OptiScalerVersion != "":
		return ui.Badge{Label: "✦ OptiScaler " + e.OptiScalerVersion, Tone: ui.TonePurple}, true
	case external:
		return ui.Badge{Label: "✦ OptiScaler · external", Tone: ui.ToneBlue}, true
	case e.Status == domain.StatusCommitted:
		return ui.Badge{Label: "✦ OptiScaler", Tone: ui.TonePurple}, true
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
	Container(Attrs(Pad2(1, 6), Corners(radiusS), BackgroundVec(bg)), func() {
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
// the trigger and popup container ids the click-outside check needs.
type dropdownState struct {
	open   bool
	btnID  ContainerId
	menuID ContainerId
}

// versionDDItem is the dropdown's observability seam: one entry per
// rendered popup row (version, current-tick, screen rect for click tests).
type versionDDItem struct {
	version string
	ticked  bool
	rect    Rect
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
	if !st.open && m.versionDDItemsFor == e.InstallDir {
		m.versionDDItems = nil
		m.versionDDItemsFor = ""
	}
	// Trigger: badgePill geometry (Pad2(1, 6), FontSize 11) so the pill row
	// height — and with it cardContentH — is untouched.
	Container(Attrs(Focusable, Row, CrossMid, Gap(sp4), Pad2(1, 6), Corners(radiusS), BackgroundVec(toneColor(tone))), func() {
		CycleFocusOnTab()
		m.ddTriggerID = CurrentId()
		if m.versionDDRects == nil {
			m.versionDDRects = map[string]Rect{}
		}
		m.versionDDRects[e.InstallDir] = GetScreenRectOf(CurrentId())
		st.btnID = CurrentId()
		activated := false
		if HasFocus() {
			m.ddFocusRing = true
			ModAttrs(func(a *AttrSet) {
				a.BorderWidth = 2
				a.BorderColor = focusBorder
			})
			if FrameInput.Key == KeyEnter || FrameInput.Key == KeySpace {
				FrameInput.Key = KeyCodeNone
				activated = true
			}
		}
		Label(label, FontSize(11), TextColor(0, 0, 96, 1))
		widgets.Icon(widgets.TypArrowSortedDown, FontSize(11), TextColor(0, 0, 96, 1))
		if PressAction() {
			activated = true
		}
		if activated {
			st.open = !st.open
			if st.open {
				m.openDropdownDir = e.InstallDir
			} else if m.openDropdownDir == e.InstallDir {
				m.openDropdownDir = ""
			}
		}
	})
	if st.open {
		dir := e.InstallDir
		current := e.OptiScalerVersion
		Popup(func() {
			// Computed here, never on closed frames: Versions walks the
			// bundle cache (see the I/O note above).
			versions := m.sess.Versions(dir)
			triggerW := GetResolvedRectOf(st.btnID).Size[0]
			Container(Attrs(MinWidth(triggerW), MaxWidth(360), Corners(radiusS), Pad2(sp4, 0), Gap(2), Clip, BackgroundVec(bgPanel), BorderWidth(1), BorderColorVec(border), elevateOverlay), func() {
				ModAttrs(FloatVec(dropdownPos(st.btnID)))
				st.menuID = CurrentId()
				m.versionDDItems = m.versionDDItems[:0]
				m.versionDDItemsFor = dir
				for _, v := range versions {
					v := v
					Container(Attrs(Row, Expand, CrossMid, Gap(sp8), Pad2(sp4, sp8), Corners(2)), func() {
						ticked := version.Compare(v, current) == 0
						m.versionDDItems = append(m.versionDDItems, versionDDItem{version: v, ticked: ticked, rect: GetScreenRectOf(CurrentId())})
						if IsHovered() {
							ModAttrs(BackgroundVec(accentHov))
						}
						// Fixed-width tick column keeps version labels aligned.
						tick := " "
						if ticked {
							tick = "✓"
						}
						Label(tick, FontSize(12), TextColorVec(txtMain))
						Label(v, FontSize(12), TextColorVec(txtMain))
						if PressAction() {
							if version.Compare(v, current) != 0 {
								m.dispatchSwitchVersion(dir, v)
							}
							st.open = false // close either way (S13 no-op on current)
						}
					})
				}
			})
		})
	}
	// Dismissal, AFTER the popup rendered so clicks inside it still register
	// (menu.go:84's ordering): Esc closes without dispatch and is consumed
	// so the global Esc handler cannot also close the detail panel; a click
	// outside both trigger and popup closes without dispatch.
	if st.open && FrameInput.Key == KeyEscape {
		FrameInput.Key = KeyCodeNone
		st.open = false
		m.openDropdownDir = ""
	}
	if st.open && !IdIsHovered(st.btnID) && !IdIsHovered(st.menuID) && FrameInput.Mouse == MouseClick {
		st.open = false
		m.openDropdownDir = ""
	}
}

// dropdownPos anchors the popup below the trigger, clamped to the window —
// a local copy of widgets/menu.go's unexported _getPositionRelativeTo.
func dropdownPos(anchorID ContainerId) Vec2 {
	targetRect := GetResolvedRectOf(anchorID)
	const sp = 4
	pos := targetRect.Origin
	pos[1] += targetRect.Size[1] + sp
	selfSize := GetResolvedSize()
	if pos[0]+selfSize[0] > WindowSize[0] {
		pos[0] = WindowSize[0] - selfSize[0] - sp
	}
	if pos[1]+selfSize[1] > WindowSize[1] {
		pos[1] = WindowSize[1] - selfSize[1] - sp
	}
	pos[0] = max(0, pos[0])
	pos[1] = max(0, pos[1])
	return pos
}
