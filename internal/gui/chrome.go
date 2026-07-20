package gui

import (
	"fmt"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// modal is widgets.Modal with a dark card: upstream hardcodes a white
// surface, which clashes with the theme.
func modal(width float32, dismiss func(), fn func()) {
	Popup(func() {
		var cardID ContainerId
		var cardFirst bool
		Container(Attrs(Float(0, 0), FixWidth(WindowSize[0]), FixHeight(WindowSize[1]), FocusTrap, Center, Background(220, 25, 12, 0.45), NoAnimate, InFront), func() {
			Container(Attrs(FixWidth(width), Gap(sp12), Pad(sp24), BackgroundVec(bgPanel), Corners(radiusL), elevateOverlay), func() {
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
	Container(Attrs(FixSize(sidebarW, 0), BackgroundVec(bgPanel), Pad(sp8), Gap(sp12)), func() {
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
		s := m.sess.Settings()
		m.versionBuf = s.DefaultVersion
		m.templateBuf = s.LaunchTemplate
	}
	m.settingsOpen = true
}

// sectionTitle heads a settings group.
func sectionTitle(s string) {
	Label(s, FontSize(13), TextColorVec(txtMain), FontWeight(WeightBold))
}

func (m *model) settingsModal() {
	modal(settingsModalW, func() { m.settingsOpen = false }, func() {
		Container(Attrs(Gap(sp16), BackgroundVec(bgPanel)), func() {
			Label("Settings", FontSize(18), TextColorVec(txtMain), FontWeight(WeightBold))

			Container(Attrs(Gap(sp4)), func() {
				sectionTitle("General")
				muted("Default OptiScaler version (tag or 'latest')")
				TextInputExt(&m.versionBuf, settingsInputAttrs())
			})

			Container(Attrs(Gap(sp4)), func() {
				sectionTitle("Scan Directories")
				m.settingsDirsSection()
			})

			Container(Attrs(Gap(sp4)), func() {
				sectionTitle("Launch Template")
				muted("Command template for manually added games; {exe} and {args} are substituted")
				TextInputExt(&m.templateBuf, settingsInputAttrs())
			})

			if m.sess != nil {
				Container(Attrs(Row, Gap(sp8)), func() {
					if focusableButton(SymIRight, "Apply") {
						m.applySettings()
					}
					if focusableButton(SymIRight, "Clear OptiScaler cache") {
						m.sess.ClearBundleCache()
					}
				})
			}
			if focusableButton(SymILeft, "Close") {
				m.settingsOpen = false
			}
		})
	})
}

// settingsInputAttrs is the shared settings-field style: no auto-focus grab
// (the modal opens on demand), theme accent underline, bounded width.
func settingsInputAttrs() TextInputAttrs {
	a := DefaultTextInputAttrs()
	a.NoAutoFocus = true
	a.Accent = accent
	a.MinWidth = 260
	a.MaxWidth = 460
	return a
}

// settingsDirsSection lists the session's extra scan directories with a
// per-row remove button and the add-directory picker entry point.
func (m *model) settingsDirsSection() {
	dirs := m.settingsDirs()
	if len(dirs) == 0 {
		muted("No extra directories — Steam library folders are always scanned")
	}
	if len(dirs) > 0 {
		Container(Attrs(Expand, MaxHeight(160), Viewport, Clip, Gap(sp4)), func() {
			for _, d := range dirs {
				Container(Attrs(Row, CrossMid, Gap(sp8), Pad2(2, sp4), Corners(radiusS), BackgroundVec(bgCard)), func() {
					Label(d, TextColorVec(txtMain), FontSize(12))
					Filler(1)
					if m.sess != nil && focusableButton(TypCancel, "Remove") {
						m.sess.RemoveDirectory(d)
					}
				})
			}
			ScrollBars()
		})
	}
	if m.sess != nil && focusableButton(SymIPlus, "Add directory…") {
		m.sess.PickAndAddDirectory(m.ctx)
	}
}

// toolbar is the top action bar: scan, add, search, sort, view switch.
func (m *model) toolbar() {
	Container(Attrs(Expand, Row, CrossMid, Gap(sp8), Pad2(sp8, sp4)), func() {
		if m.sess != nil {
			busy := m.state.Busy != ""
			if focusableButtonExt("Scan", ButtonAttrs{Icon: SymRefresh, Disabled: busy}) && !busy {
				m.sess.Scan(m.ctx)
			}
			if busy {
				spinnerGlyph()
			}
		}
		if m.sess != nil && focusableButton(SymIPlus, "Add Game") {
			m.sess.PickAndAddDirectory(m.ctx)
		}
		Container(Attrs(Grow(1), MinSize(140, 34), MaxSizeVec(Vec2{420, 34})), func() {
			TextInputExt(&m.filter, searchInputAttrs())
		})
		if m.sess != nil {
			MenuButtonExt("Sort: "+sortLabel(m.state.Sort), ButtonAttrs{Icon: TypArrowSortedDown}, func() {
				if MenuItem(SymStar, "Default (actionable first)") {
					m.setSort(ui.SortDefault)
				}
				if MenuItem(0, "Name (A–Z)") {
					m.setSort(ui.SortName)
				}
			})
			m.viewSwitch()
		}
		Filler(1)
		if m.state.Busy != "" {
			muted(m.state.Busy)
		}
	})
}

// searchInputAttrs styles the library search field.
func searchInputAttrs() TextInputAttrs {
	a := DefaultTextInputAttrs()
	a.NoAutoFocus = true
	a.Accent = accent
	return a
}

// sortLabel is the toolbar caption for the active sort mode.
func sortLabel(mode ui.SortMode) string {
	if mode == ui.SortName {
		return "Name"
	}
	return "Default"
}

// statusBar is the bottom strip with the current status line.
func (m *model) statusBar() {
	Container(Attrs(Expand, BackgroundVec(bgPanel), Pad2(sp8, sp12), Row, CrossMid, Gap(sp12)), func() {
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
	modal(aboutModalW, func() { m.about = false }, func() {
		Container(Attrs(Gap(sp8)), func() {
			txt("optiscaler-manager " + m.cfg.Version)
			muted("OptiScaler manager for local games — Linux + Steam.")
			muted("GUI: go-shirei (pinned v0.5.2)")
			if focusableButton(SymILeft, "Close") {
				m.about = false
			}
		})
	})
}
