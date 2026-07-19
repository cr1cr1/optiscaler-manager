package gui

import (
	"context"
	"fmt"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"
)

// sidebar is the icon navigation column on the left edge.
func (m *model) sidebar() {
	Container(Attrs(FixSize(64, 0), BackgroundVec(bgPanel), Pad(8), Gap(12)), func() {
		Label("✦", FontSize(22), TextColorVec(toneColor(2)))
		if Button(0, "Games") {
			m.about = false
		}
		if Button(0, "About") {
			m.about = true
		}
	})
}

// toolbar is the top action bar: scan, search, view switch.
func (m *model) toolbar(ctx context.Context) {
	Container(Attrs(Expand, Row, CrossMid, Gap(10), Pad2(8, 4)), func() {
		if m.sess != nil && Button(SymIRight, "Scan Games") {
			m.sess.Scan(ctx)
		}
		Container(Attrs(FixSize(320, 34)), func() {
			TextInput(&m.filter)
		})
		if m.sess != nil && Button(SymIRight, viewToggleLabel(m.state.Mode)) {
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
	Modal(420, func() { m.about = false }, func() {
		Container(Attrs(Pad(18), Gap(8)), func() {
			txt("optiscaler-manager " + m.cfg.Version)
			muted("OptiScaler manager for local games — Linux + Steam.")
			muted("GUI: go-shirei (pinned v0.5.2)")
			if Button(SymILeft, "Close") {
				m.about = false
			}
		})
	})
}
