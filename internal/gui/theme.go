package gui

import (
	. "go.hasen.dev/shirei"
	"go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Buttons take their text color from ContrastingTextColor(ButtonAccent), so a
// dark accent yields light text automatically. DefaultBackground is shirei's
// surface color for modals and popup panels.
func init() {
	widgets.ButtonAccent = accent
	widgets.DefaultBackground = Vec4{230, 20, 16, 1}
	// Virtualized lists draw their own scrollbars with this fallback accent;
	// the default is an off-palette bright cyan.
	widgets.DefaultAccent = scrolAccent
}

// Spacing scale: every padding, gap, and offset derives from these tokens.
const (
	sp4  float32 = 4
	sp8  float32 = 8
	sp12 float32 = 12
	sp16 float32 = 16
	sp24 float32 = 24
)

// Corner radii scale.
const (
	radiusS float32 = 4
	radiusM float32 = 6
	radiusL float32 = 10
)

// Chrome widths and the shared modal widths, exported for layout tests.
const (
	sidebarW       = 72
	settingsModalW = 560
	confirmModalW  = 460
	aboutModalW    = 420
)

// Detail panel sizing: 30% of the window, clamped so narrow windows keep a
// usable panel and ultrawide windows cap the sidebar.
const (
	detailPanelMinW = 300
	detailPanelMaxW = 480
)

// detailPanelWidth is the proportional width of the right-docked detail
// panel for a given window width.
func detailPanelWidth(windowW float32) float32 {
	w := windowW * 0.30
	if w < detailPanelMinW {
		return detailPanelMinW
	}
	if w > detailPanelMaxW {
		return detailPanelMaxW
	}
	return w
}

// Dark theme palette (HSLA), matching the reference client's look.
var (
	bgApp       = Vec4{230, 25, 11, 1}
	bgPanel     = Vec4{230, 20, 16, 1}
	bgCard      = Vec4{230, 18, 20, 1}
	bgRaised    = Vec4{230, 16, 25, 1}
	border      = Vec4{230, 12, 32, 1}
	accent      = Vec4{220, 18, 26, 1}
	accentHov   = Vec4{220, 22, 34, 1}
	focusBorder = Vec4{210, 70, 62, 1} // bright: focus rings and the text caret
	selBg       = Vec4{210, 45, 45, 1} // selection highlight band
	scrolAccent = Vec4{220, 10, 45, 1}
	txtMain     = Vec4{220, 15, 92, 1}
	txtMuted    = Vec4{220, 10, 62, 1}
	txtWarn     = Vec4{20, 80, 65, 1}
)

// elevateOverlay is the stronger lift for modals, toasts, and popups.
func elevateOverlay(a *AttrSet) {
	a.Blur = 28
	a.Alpha = 0.5
	a.Offset[1] = 6
}

// scrollBars renders themed (muted gray-blue) scrollbars for the current
// scrolling container; the default accent is an off-palette bright cyan.
func scrollBars() { widgets.ScrollBarsExt(widgets.ScrollBarsAttrs{Accent: scrolAccent}) }

// txt and muted are the standard body texts.
func txt(s string)   { Label(s, TextColorVec(txtMain)) }
func muted(s string) { Label(s, TextColorVec(txtMuted), FontSize(12)) }

// toneColor maps a ui badge tone to a pill background.
func toneColor(t ui.Tone) Vec4 {
	switch t {
	case ui.ToneGreen:
		return Vec4{140, 55, 32, 1}
	case ui.ToneRed:
		return Vec4{5, 60, 40, 1}
	case ui.ToneBlue:
		return Vec4{210, 60, 42, 1}
	case ui.TonePurple:
		return Vec4{275, 45, 45, 1}
	default:
		return Vec4{220, 10, 35, 1}
	}
}

// badgePill renders a small colored pill like the client's tech badges.
func badgePill(label string, tone ui.Tone) {
	Container(Attrs(Pad2(1, 6), Corners(radiusS), BackgroundVec(toneColor(tone))), func() {
		Label(label, TextColor(0, 0, 96, 1), FontSize(11))
	})
}
