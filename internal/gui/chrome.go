package gui

import (
	"context"
	"fmt"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"
)

// modal is widgets.Modal with a dark card: upstream hardcodes a white
// surface, which clashes with the theme.
func modal(width float32, dismiss func(), fn func()) {
	Popup(func() {
		var cardID ContainerId
		var cardFirst bool
		Container(Attrs(Float(0, 0), FixWidth(WindowSize[0]), FixHeight(WindowSize[1]), FocusTrap, Center, Background(220, 25, 12, 0.45), NoAnimate, InFront), func() {
			Container(Attrs(FixWidth(width), Gap(10), Pad(20), BackgroundVec(bgPanel), Corners(10), BoxShadow(24)), func() {
				cardID = CurrentId()
				cardFirst = FirstRender()
				fn()
			})
			if dismiss != nil && FrameInput.Key == KeyEscape {
				dismiss()
			}
			if dismiss != nil && !cardFirst && FrameInput.Mouse == MouseClick && !IdIsHovered(cardID) {
				dismiss()
			}
		})
	})
}

// sidebar is the icon navigation column on the left edge.
func (m *model) sidebar() {
	Container(Attrs(FixSize(64, 0), BackgroundVec(bgPanel), Pad(8), Gap(12)), func() {
		Label("✦", FontSize(22), TextColorVec(toneColor(2)))
		if focusableButton(0, "Games") {
			m.about = false
			m.settingsOpen = false
		}
		if focusableButton(0, "Settings") {
			m.about = false
			m.openSettings()
		}
		if focusableButton(0, "About") {
			m.settingsOpen = false
			m.about = true
		}
		if focusableButton(0, "Exit") {
			m.exit()
		}
	})
}

// openSettings primes the settings modal from the session.
func (m *model) openSettings() {
	if m.sess != nil {
		m.versionBuf = m.sess.Settings().DefaultVersion
	}
	m.settingsOpen = true
}

func (m *model) settingsModal() {
	modal(480, func() { m.settingsOpen = false }, func() {
		Container(Attrs(Pad(18), Gap(10), BackgroundVec(bgPanel)), func() {
			txt("Settings")
			muted("Default OptiScaler version (tag or 'latest')")
			TextInput(&m.versionBuf)
			if m.sess == nil {
				return
			}
		if focusableButton(SymIRight, "Apply") {
			m.sess.SetDefaultVersion(m.versionBuf)
		}
		if focusableButton(SymIRight, "Clear OptiScaler cache") {
			m.sess.ClearBundleCache()
		}
		if focusableButton(SymILeft, "Close") {
			m.settingsOpen = false
		}
		})
	})
}

// toolbar is the top action bar: scan, search, view switch.
func (m *model) toolbar(ctx context.Context) {
	Container(Attrs(Expand, Row, CrossMid, Gap(10), Pad2(8, 4)), func() {
		if m.sess != nil && focusableButton(SymIRight, "Scan Games") {
			m.sess.Scan(ctx)
		}
		if m.sess != nil && focusableButton(SymIPlus, "Add Game") {
			m.sess.PickAndAddDirectory(ctx)
		}
		Container(Attrs(Grow(1), MinSize(140, 34), MaxSizeVec(Vec2{420, 34})), func() {
			TextInput(&m.filter)
		})
		if m.sess != nil && focusableButton(SymIRight, viewToggleLabel(m.state.Mode)) {
			m.sess.ToggleView()
		}
		Filler(1)
		if m.state.Busy != "" {
			muted(m.state.Busy)
		}
	})
}

// statusBar is the bottom strip with the current status line.
func (m *model) statusBar() {
	Container(Attrs(Expand, BackgroundVec(bgPanel), Pad2(6, 10), Row, CrossMid, Gap(10)), func() {
		muted(m.state.StatusLine)
		Filler(1)
		muted(fmt.Sprintf("%d games", len(m.state.Rows)))
	})
}

// toastOverlay floats active toasts at the bottom-right, above the list.
func (m *model) toastOverlay() {
	if len(m.state.Toasts) == 0 {
		return
	}
	y := WindowSize[1] - float32(24+len(m.state.Toasts)*26) - 16
	Container(Attrs(Float(WindowSize[0]-380, y), Z(10), FixSize(360, 0), BackgroundVec(bgPanel), Corners(6), Pad(10), Gap(4)), func() {
		for _, t := range m.state.Toasts {
			if t.Warn {
				Label(t.Text, TextColorVec(txtWarn), FontSize(12))
			} else {
				Label(t.Text, TextColorVec(txtMain), FontSize(12))
			}
		}
	})
}

// aboutModal shows version and project info.
func (m *model) aboutModal() {
	modal(420, func() { m.about = false }, func() {
		Container(Attrs(Pad(18), Gap(8)), func() {
			txt("optiscaler-manager " + m.cfg.Version)
			muted("OptiScaler manager for local games — Linux + Steam.")
			muted("GUI: go-shirei (pinned v0.5.2)")
		if focusableButton(SymILeft, "Close") {
			m.about = false
		}
		})
	})
}
