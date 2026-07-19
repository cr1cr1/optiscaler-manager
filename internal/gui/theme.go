package gui

import (
	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Dark theme palette (HSLA), matching the reference client's look.
var (
	bgApp    = Vec4{230, 25, 11, 1}
	bgPanel  = Vec4{230, 20, 16, 1}
	bgCard   = Vec4{230, 18, 20, 1}
	txtMain  = Vec4{220, 15, 92, 1}
	txtMuted = Vec4{220, 10, 62, 1}
	txtWarn  = Vec4{20, 80, 65, 1}
)

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
	Container(Attrs(Pad2(1, 6), Corners(4), BackgroundVec(toneColor(tone))), func() {
		Label(label, TextColor(0, 0, 96, 1), FontSize(11))
	})
}
