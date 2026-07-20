package gui

import (
	"strings"
	"time"

	. "go.hasen.dev/shirei"
	"go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
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
				a.BorderColor = Vec4{210, 70, 62, 1}
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

// searchInput is the themed library filter field. Disabled while the
// library is empty.
func (m *model) searchInput() {
	if m.libraryEmpty() {
		Container(Attrs(Corners(radiusM), BackgroundVec(bgRaised), BorderWidth(1), BorderColorVec(border), Pad2(sp8, sp12), Grow(1), MinSize(140, 34), MaxSizeVec(Vec2{420, 34}), Clip, Trans(0.4)), func() {
			Container(Attrs(Row, Gap(sp8), CrossMid), func() {
				widgets.Icon(widgets.SymSearch, FontSize(13), TextColorVec(txtMuted))
				Label("Search…", FontSize(13), TextColorVec(txtMuted))
			})
		})
		return
	}
	themedInput(&m.filter, "Search…", widgets.SymSearch, Grow(1), MinSize(140, 34), MaxSizeVec(Vec2{420, 34}))
}

// themedInput is the dark single-line text field: shirei's TextInputExt is
// hardcoded light-on-white with no theme hook, so themed screens use this
// minimal input instead. Editing is append/backspace on the bound buffer,
// Esc clears and blurs, Enter is consumed so it cannot leak to global key
// handlers. It never grabs focus on its own (FocusOnClick + the Tab cycle
// only), so modals open with nothing focused. The icon is optional (0 for
// none); sizing attrs set the field's extent.
func themedInput(buf *string, hint string, icon rune, sizing ...AttrsFn) {
	box := Attrs(Focusable, Corners(radiusM), BackgroundVec(bgRaised), BorderWidth(1), BorderColorVec(border), Pad2(sp8, sp12), Clip)
	Container(AttrsWith(box, sizing...), func() {
		CycleFocusOnTab()
		FocusOnClick()
		if HasFocus() {
			ModAttrs(func(a *AttrSet) { a.BorderColor = accent })
			*buf += FrameInput.Text
			switch FrameInput.Key {
			case KeyDeleteBackward:
				if r := []rune(*buf); len(r) > 0 {
					*buf = string(r[:len(r)-1])
				}
				FrameInput.Key = KeyCodeNone
			case KeyEscape:
				*buf = ""
				Blur()
				FrameInput.Key = KeyCodeNone
			case KeyEnter:
				FrameInput.Key = KeyCodeNone
			}
		}
		Container(Attrs(Row, Gap(sp8), CrossMid), func() {
			if icon != 0 {
				widgets.Icon(icon, FontSize(13), TextColorVec(txtMuted))
			}
			switch {
			case *buf != "" && HasFocus():
				Label(*buf+"|", FontSize(13), TextColorVec(txtMain))
			case *buf != "":
				Label(*buf, FontSize(13), TextColorVec(txtMain))
			case HasFocus():
				Label("|", FontSize(13), TextColorVec(txtMuted))
			default:
				Label(hint, FontSize(13), TextColorVec(txtMuted))
			}
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

// versionPills is the install-version badge set for a row: the OptiScaler
// pill (versioned when the installed version is known), one pill per
// component version, and a Proton marker for prefixed games.
func versionPills(e *ui.GameRow) []ui.Badge {
	var out []ui.Badge
	if e.OptiScalerVersion != "" {
		out = append(out, ui.Badge{Label: "✦ OptiScaler " + e.OptiScalerVersion, Tone: ui.TonePurple})
	} else if e.Status == domain.StatusCommitted {
		out = append(out, ui.Badge{Label: "✦ OptiScaler", Tone: ui.TonePurple})
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

// launchable reports whether a row carries enough identity to launch:
// store games go by AppID, manual/GOG games by executable path.
func launchable(e *ui.GameRow) bool {
	return e.AppID != "" || e.ExePath != ""
}
