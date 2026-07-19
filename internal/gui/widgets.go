package gui

import (
	"strings"

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
		if widgets.Button(icon, label) {
			activated = true
		}
	})
	return activated
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
