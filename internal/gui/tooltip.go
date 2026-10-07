package gui

import (
	"strings"
	"time"

	. "go.hasen.dev/shirei"
)

// Issue 019: pill tooltips. Hovering a pill whose label leads with a known
// abbreviation floats the abbreviation's full name next to the pill.
// shirei has no tooltip primitive, so the pill registers the request during
// the frame and rootView renders the overlay last — outside every Clip and
// virtualized viewport a pill may live in.

// pillTipDelay is the hover debounce: the tooltip appears only after the
// pointer has rested on the same pill for half a second, so sweeping the
// mouse across a pill row doesn't strobe overlays.
const pillTipDelay = 500 * time.Millisecond

// pillTipState tracks the pending tooltip across frames: the hovered pill's
// full-name text and screen rect (the overlay's anchor), when the hover on
// THAT pill started (the debounce clock), whether any pill registered this
// frame (seen), and whether the overlay actually rendered (shown).
type pillTipState struct {
	text  string
	rect  Rect
	since time.Time
	seen  bool
	shown bool
}

// pillTip is re-armed by rootView every frame (seen/shown cleared) and set
// by pillHoverTip as pills render; text/rect/since persist across frames so
// the debounce clock survives. Package-level because badgePill — the shared
// pill renderer — has no model handle; the GUI is a single instance
// rendering single-threaded.
var pillTip pillTipState

// pillTipFullNames maps the pill label's leading token to its full name.
// Keys match the classified upscaler kinds (domain/release.go) plus EAC;
// plain names (Proton, Steam, fork names) and unknown tokens get no tooltip.
var pillTipFullNames = map[string]string{
	"DLSS":    "NVIDIA Deep Learning Super Sampling",
	"DLSS-FG": "NVIDIA DLSS Frame Generation",
	"FSR":     "AMD FidelityFX Super Resolution",
	"XeSS":    "Intel Xe Super Sampling",
	"EAC":     "Easy Anti-Cheat",
}

// pillTipText resolves a pill label to its tooltip text: the label's first
// token (version suffix stripped) must be a known abbreviation.
func pillTipText(label string) string {
	token, _, _ := strings.Cut(label, " ")
	return pillTipFullNames[token]
}

// pillHoverTip registers the frame's tooltip when the current pill container
// is hovered and its label carries a known abbreviation. Call it inside the
// pill's container closure so CurrentId is the pill. Hovering the SAME pill
// keeps the debounce clock running; moving to a different pill restarts it.
func pillHoverTip(label string) {
	if !IsHovered() {
		return
	}
	tip := pillTipText(label)
	if tip == "" {
		return
	}
	r := GetScreenRectOf(CurrentId())
	if pillTip.text != tip || pillTip.rect != r {
		pillTip.since = time.Now()
	}
	pillTip.text, pillTip.rect, pillTip.seen = tip, r, true
}

// pillTipOverlay renders the pending tooltip below its anchor pill (above it
// when the pill sits near the window's bottom), clamped into the window,
// once the hover debounce has elapsed. Runs at the end of rootView so it
// paints over cards, panel, and toasts. When no pill registered this frame
// the state clears, so the next hover debounces from zero again.
func (m *model) pillTipOverlay() {
	if !pillTip.seen {
		pillTip = pillTipState{}
		return
	}
	if time.Since(pillTip.since) < pillTipDelay {
		return
	}
	pillTip.shown = true
	const (
		padH   = 8
		padV   = 4
		margin = 8
		gap    = 4
	)
	tipW := textWidthAt(pillTip.text, 11) + 2*padH
	tipH := float32(11+2*padV) + 2 // text line + padding, plus breathing room
	win := GetHost().WindowSize
	x := pillTip.rect.Origin[0]
	if x+tipW > win[0]-margin {
		x = win[0] - tipW - margin
	}
	if x < margin {
		x = margin
	}
	y := pillTip.rect.Origin[1] + pillTip.rect.Size[1] + gap
	if y+tipH > win[1]-margin {
		y = pillTip.rect.Origin[1] - tipH - gap // flip above the pill
	}
	Container(Attrs(Float(x, y), Z(20), Pad2(padV, padH), Corners(radiusS),
		BackgroundVec(bgRaised), BorderWidth(1), BorderColorVec(border), elevateOverlay), func() {
		Label(pillTip.text, FontSize(11), TextColorVec(txtMain))
	})
}
