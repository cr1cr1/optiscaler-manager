package gui

import (
	"fmt"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// modal is widgets.Modal with a dark card: upstream hardcodes a white
// surface, which clashes with the theme.
func modal(width float32, dismiss func(), fn func()) {
	Popup(func() {
		var cardID ContainerId
		var cardFirst bool
		Container(Attrs(Float(0, 0), FixWidth(GetHost().WindowSize[0]), FixHeight(GetHost().WindowSize[1]), FocusTrap, Center, Background(220, 25, 12, 0.45), NoAnimate, InFront), func() {
			Container(Attrs(FixWidth(width), Gap(sp12), Pad(sp24), BackgroundVec(bgPanel), Corners(radiusL), elevateOverlay), func() {
				cardID = CurrentId()
				cardFirst = FirstRender()
				fn()
			})
			if dismiss != nil && GetFrameInput().Key == KeyEscape {
				dismiss()
			}
			if dismiss != nil && !cardFirst && GetFrameInput().Mouse == MouseClick && !IdIsHovered(cardID) {
				dismiss()
			}
		})
	})
}

// sidebar is the icon navigation column on the left edge: it fills the
// full window height, the nav group (glyph above a short label) is centered
// vertically, and Exit stays pinned to the bottom; the active section is
// highlighted in the accent color.
func (m *model) sidebar() {
	m.sidebarRects = m.sidebarRects[:0]
	Container(Attrs(FixSize(sidebarW, GetHost().WindowSize[1]), BackgroundVec(bgPanel), Pad(sp8), Gap(sp12)), func() {
		m.sidebarShellRect = GetScreenRectOf(CurrentId())
		Container(Attrs(Row, Center, Expand), func() {
			Label("✦", FontSize(22), TextColorVec(toneColor(2)))
		})
		Filler(1)
		m.sidebarItem(SymHome, "Games", !m.about && !m.settingsOpen, func() {
			m.about = false
			m.settingsOpen = false
		})
		m.sidebarItem(SymCog, "Prefs", m.settingsOpen, func() {
			m.about = false
			m.openSettings()
		})
		m.sidebarItem(SymInfo, "About", m.about, func() {
			m.settingsOpen = false
			m.about = true
		})
		Filler(1)
		m.sidebarItem(SymPower, "Exit", false, m.exit)
	})
}

// sidebarItem is one navigation entry: icon over a truncated label, tinted
// with the accent while its section is active.
func (m *model) sidebarItem(icon IconGlyph, label string, active bool, action func()) {
	fg := txtMuted
	if active {
		fg = accentHov
	}
	Container(Attrs(PointerHand, Center, Expand, Gap(2), Pad2(sp4, 2), Corners(radiusS)), func() {
		m.sidebarRects = append(m.sidebarRects, GetScreenRectOf(CurrentId()))
		if !active && IsHovered() {
			ModAttrs(BackgroundVec(bgRaised))
		}
		Icon(icon, FontSize(20), TextColorVec(fg))
		Label(label, FontSize(10), TextColorVec(fg))
		if PressAction() {
			action()
		}
	})
}

// openSettings primes the settings modal from the session.
func (m *model) openSettings() {
	if m.sess != nil {
		s := m.sess.Settings()
		m.versionBuf = s.DefaultVersion
		m.templateBuf = s.LaunchTemplate
		m.onlineBuf = s.OnlineLookups
		m.umuEnabledBuf = s.UmuEnabled
		m.umuProtonBuf = s.UmuProtonPath
	}
	m.settingsTab = settingsTabGeneral
	m.settingsOpen = true
}

// sectionTitle heads a settings group.
func sectionTitle(s string) {
	Label(s, FontSize(13), TextColorVec(txtMain), FontWeight(WeightBold))
}

// settingsTab identifies the settings modal's tabbed pages: General holds
// library/launch settings, Optiscaler everything OptiScaler-specific.
type settingsTab int

const (
	settingsTabGeneral settingsTab = iota
	settingsTabOptiscaler
)

func (m *model) settingsModal() {
	modal(settingsModalW, func() { m.settingsOpen = false }, func() {
		Container(Attrs(Expand, Gap(sp16), BackgroundVec(bgPanel)), func() {
			Label("Settings", FontSize(18), TextColorVec(txtMain), FontWeight(WeightBold))
			m.settingsTabBar()
			if m.settingsTab == settingsTabOptiscaler {
				m.settingsOptiscalerTab()
			} else {
				m.settingsGeneralTab()
			}

			if m.sess != nil && focusableButton(SymIRight, "Apply") {
				m.applySettings()
			}
			if focusableButton(SymILeft, "Close") {
				m.settingsOpen = false
			}
		})
	})
}

// settingsTabBar is the modal's tab strip: a recessed track with one pill
// per tab; the active tab is raised with a bright bold label, the inactive
// one muted (hover raises it).
func (m *model) settingsTabBar() {
	Container(Attrs(Row, Gap(2), Pad(2), Corners(radiusS), BackgroundVec(bgCard)), func() {
		m.settingsTabButton(settingsTabGeneral, "General")
		m.settingsTabButton(settingsTabOptiscaler, "Optiscaler")
	})
}

// settingsTabButton is one tab in the strip. A click both activates the
// tab and moves keyboard focus to it (FocusOnClick — the focus ring is
// the selection read-out, so it must follow the click); Enter/Space
// activate when focused and Left/Right move between tabs (the global
// arrow handlers are muted while the modal is open).
func (m *model) settingsTabButton(tab settingsTab, label string) {
	active := m.settingsTab == tab
	Container(Attrs(Focusable, PointerHand, Center, Pad2(sp4, sp12), Corners(radiusS)), func() {
		FocusOnClick()
		if m.settingsTabIDs == nil {
			m.settingsTabIDs = map[settingsTab]ContainerId{}
		}
		m.settingsTabIDs[tab] = CurrentId()
		if m.settingsTabRects == nil {
			m.settingsTabRects = map[settingsTab]Rect{}
		}
		m.settingsTabRects[tab] = GetScreenRectOf(CurrentId())
		if active || IsHovered() {
			ModAttrs(BackgroundVec(bgRaised))
		}
		if HasFocus() {
			ModAttrs(func(a *AttrSet) {
				a.BorderWidth = 2
				a.BorderColor = focusBorder
			})
			switch GetFrameInput().Key {
			case KeyEnter, KeySpace:
				GetFrameInput().Key = KeyCodeNone
				m.settingsTab = tab
			case KeyRight:
				GetFrameInput().Key = KeyCodeNone
				m.settingsTab = settingsTabOptiscaler
			case KeyLeft:
				GetFrameInput().Key = KeyCodeNone
				m.settingsTab = settingsTabGeneral
			}
		}
		if active {
			// txtMain, not accentHov: the accent (lightness 34) is too dim
			// against bgRaised to read as the selected state.
			Label(label, FontSize(12), TextColorVec(txtMain), FontWeight(WeightBold))
		} else {
			Label(label, FontSize(12), TextColorVec(txtMuted))
		}
		if PressAction() {
			m.settingsTab = tab
		}
	})
}

// settingsGeneralTab: library view, scan directories, launch template, and
// the umu-launcher section.
func (m *model) settingsGeneralTab() {
	Container(Attrs(Expand, Gap(sp16)), func() {
		Container(Attrs(Expand, Gap(sp4)), func() {
			sectionTitle("Library")
			if m.sess != nil {
				focusableToggle(&m.onlineBuf, "Online game info (Steam/ProtonDB)")
				if m.onlineBuf != m.sess.Settings().OnlineLookups {
					m.sess.SetOnlineLookups(m.onlineBuf)
				}
				m.cardSizeSelector()
			}
		})

		Container(Attrs(Expand, Gap(sp4)), func() {
			sectionTitle("Scan Directories")
			m.settingsDirsSection()
		})

		Container(Attrs(Expand, Gap(sp4)), func() {
			sectionTitle("Launch Template")
			muted("Command template for manually added games; {exe} and {args} are substituted")
			themedInput(&m.templateBuf, `"{exe}" {args}`, NoIcon, MinSize(260, fieldH), MaxSizeVec(Vec2{460, fieldH}))
		})

		Container(Attrs(Expand, Gap(sp4)), func() {
			sectionTitle("umu-launcher (Linux: Windows games via Proton)")
			if m.sess != nil {
				focusableToggle(&m.umuEnabledBuf, "Launch Windows binaries via umu-launcher")
				if m.umuEnabledBuf != m.sess.Settings().UmuEnabled {
					m.sess.SetUmuEnabled(m.umuEnabledBuf)
				}
				muted("Proton path (blank = auto-detect from Steam / Bottles / umu)")
				themedInput(&m.umuProtonBuf, "", NoIcon, MinSize(260, fieldH), MaxSizeVec(Vec2{460, fieldH}))
				if m.umuProtonBuf != m.sess.Settings().UmuProtonPath {
					m.sess.SetUmuProtonPath(m.umuProtonBuf)
				}
			}
		})
	})
}

// settingsOptiscalerTab: everything OptiScaler-specific — the default
// version, the distribution sources (forks), and the bundle cache.
func (m *model) settingsOptiscalerTab() {
	Container(Attrs(Expand, Gap(sp16)), func() {
		Container(Attrs(Expand, Gap(sp4)), func() {
			sectionTitle("Version")
			muted("Default OptiScaler version (tag or 'latest')")
			themedInput(&m.versionBuf, "latest", NoIcon, MinSize(260, fieldH), MaxSizeVec(Vec2{460, fieldH}))
		})

		Container(Attrs(Expand, Gap(sp4)), func() {
			sectionTitle("Sources")
			muted("Distribution fork installs download from (owner/repo + asset glob)")
			m.settingsForksSection()
		})

		if m.sess != nil {
			Container(Attrs(Expand, Gap(sp4)), func() {
				sectionTitle("Cache")
				if focusableButton(SymIRight, "Clear OptiScaler cache") {
					m.sess.ClearBundleCache()
				}
			})
		}
	})
}

// settingsDirsSection lists the session's extra scan directories with a
// per-row remove button and the add-directory picker entry point.

// settingsForksSection lists the known OptiScaler distribution forks: the
// active one is marked, a row's "Use" button selects it through the
// session, custom forks get a Remove button (upstream never does), and two
// inputs plus an Add button register new forks.
func (m *model) settingsForksSection() {
	if m.sess == nil {
		return
	}
	s := m.sess.Settings()
	for _, f := range s.Forks {
		fork := f
		active := fork.Slug == s.ActiveFork
		// Two-line row: long slugs (the bundled DLSSNR fork is 45 chars)
		// plus the pattern and buttons never fit one line in the modal.
		Container(Attrs(Expand, Gap(1), Pad2(sp4, sp8), Corners(radiusS), BackgroundVec(bgCard), Clip), func() {
			Container(Attrs(Row, Expand, CrossMid, Gap(sp8)), func() {
				label := fork.Slug
				if active {
					label = "● " + label
				}
				Label(label, TextColorVec(txtMain), FontSize(12))
				Filler(1)
				if active {
					muted("in use")
				} else if focusableButton(SymIRight, "Use") {
					_ = m.sess.SetActiveFork(fork.Slug)
				}
				if fork.Slug != settings.DefaultForkSlug && focusableButton(TypCancel, "Remove") {
					m.removeFork(fork.Slug)
				}
			})
			muted(fork.AssetPattern)
		})
	}
	Container(Attrs(Row, Expand, CrossMid, Gap(sp8)), func() {
		themedInput(&m.forkSlugBuf, "owner/repo", NoIcon, MinSize(180, fieldH), MaxSizeVec(Vec2{220, fieldH}))
		themedInput(&m.forkPatternBuf, "asset glob, e.g. OptiScaler*.zip", NoIcon, MinSize(180, fieldH), MaxSizeVec(Vec2{220, fieldH}))
		if focusableButton(SymIPlus, "Add & use") {
			m.addForkFromBuffers()
		}
	})
}

// addForkFromBuffers registers the fork described by the add inputs and
// makes it the active source (add & use). A successful add clears the
// buffers; a refusal keeps them for editing (the session toasts the
// reason).
func (m *model) addForkFromBuffers() {
	fork := settings.Fork{Slug: m.forkSlugBuf, AssetPattern: m.forkPatternBuf}
	if err := m.sess.AddFork(fork); err != nil {
		return
	}
	_ = m.sess.SetActiveFork(fork.Slug)
	m.forkSlugBuf = ""
	m.forkPatternBuf = ""
}

// removeFork deletes a custom fork through the session (refusals toast).
func (m *model) removeFork(slug string) {
	_ = m.sess.RemoveFork(slug)
}

// cardSizeSelector renders the Small/Medium/Large card-size segmented control.
func (m *model) cardSizeSelector() {
	cur := m.sess.Settings().CardSize
	muted("Card size")
	Container(Attrs(Row, Gap(sp4)), func() {
		for _, opt := range []settings.CardSize{settings.CardSizeSmall, settings.CardSizeMedium, settings.CardSizeLarge} {
			selected := opt == cur
			label := string(opt)
			if selected {
				label = "● " + label
			}
			if focusableButtonExt(label, ButtonAttrs{Icon: NoIcon}) && !selected {
				m.sess.SetCardSize(opt)
				m.cardSize = opt
			}
		}
	})
}
func (m *model) settingsDirsSection() {
	dirs := m.settingsDirs()
	if len(dirs) == 0 {
		muted("No extra directories — Steam library folders are always scanned")
	}
	if len(dirs) > 0 {
		// A Viewport inside the auto-sized modal needs an explicit height:
		// content-driven sizing would collapse it to zero and clip the rows.
		h := float32(len(dirs))*32 + sp8
		if h > 160 {
			h = 160
		}
		Container(Attrs(Expand, FixHeight(h), Viewport, Clip, Gap(sp4)), func() {
			for _, d := range dirs {
				Container(Attrs(Row, Expand, CrossMid, Gap(sp8), Pad2(2, sp4), Corners(radiusS), BackgroundVec(bgCard), Clip), func() {
					Label(shortenPath(d, 44), TextColorVec(txtMain), FontSize(12))
					Filler(1)
					if m.sess != nil && focusableButton(TypCancel, "Remove") {
						m.sess.RemoveDirectory(d)
					}
				})
			}
			scrollBars()
		})
	}
	if m.sess != nil && focusableButton(SymIPlus, "Add directory…") {
		m.sess.PickAndAddDirectory(m.ctx)
	}
}

// toolbar is the top action bar: scan, add, search, sort, view switch.
func (m *model) toolbar() {
	Container(Attrs(Expand, Row, CrossMid, Gap(sp8), Pad2(sp16, sp16)), func() {
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
		m.searchInput()
		if m.sess != nil {
			m.sortDropdown()
			m.viewSwitch()
		}
		Filler(1)
		if m.state.Busy != "" {
			muted(m.state.Busy)
		}
	})
}

// progressBar is the thin scan-pipeline indicator under the toolbar: a
// phase label and a weighted fill tracking Done/Total. Hidden while no scan
// runs (State.Progress is nil). shirei has no progress widget, so the bar
// is hand-rolled from two Grow-weighted containers.
func (m *model) progressBar() {
	m.progressTrackRect = Rect{}
	m.progressFillRect = Rect{}
	p := m.state.Progress
	if p == nil {
		return
	}
	var frac float32
	if p.Total > 0 {
		frac = float32(p.Done) / float32(p.Total)
	}
	if frac > 1 {
		frac = 1
	}
	Container(Attrs(Expand, Row, CrossMid, Gap(sp8), Pad2(sp8, 2)), func() {
		Label(fmt.Sprintf("%s %d/%d", p.Phase, p.Done, p.Total), FontSize(11), TextColorVec(txtMuted))
		Container(Attrs(Row, Grow(1), FixHeight(6), Corners(3), BackgroundVec(bgRaised), Clip), func() {
			m.progressTrackRect = GetScreenRectOf(CurrentId())
			if frac > 0 {
				Container(Attrs(Grow(frac), Expand, BackgroundVec(accentHov)), func() {
					m.progressFillRect = GetScreenRectOf(CurrentId())
				})
			}
			if frac < 1 {
				Container(Attrs(Grow(1-frac), Expand), func() {})
			}
		})
	})
}

// sortLabel is the toolbar caption for the active sort mode.
func sortLabel(mode ui.SortMode) string {
	if mode == ui.SortName {
		return "Name"
	}
	return "Default"
}

// statusBar is the bottom strip with the current status line, the keyboard
// shortcut hints, and the library size.
func (m *model) statusBar() {
	Container(Attrs(Expand, BackgroundVec(bgPanel), Pad2(sp8, sp12), Row, CrossMid, Gap(sp12)), func() {
		muted(m.state.StatusLine)
		Filler(1)
		muted("Tab: focus · ←→↑↓: select · Enter: open · Esc: close")
		muted(fmt.Sprintf("%d games", len(m.state.Rows)))
	})
}

// toastOverlay floats the newest toasts (at most three) at the bottom-right
// as raised cards with a tone accent bar.
func (m *model) toastOverlay() {
	toasts := m.state.Toasts
	if len(toasts) == 0 {
		return
	}
	if len(toasts) > 3 {
		toasts = toasts[len(toasts)-3:]
	}
	const toastH = 40 // card padding + one text line + gap, per toast
	y := GetHost().WindowSize[1] - float32(len(toasts)*toastH) - sp24
	Container(Attrs(Float(GetHost().WindowSize[0]-380, y), Z(10), FixSize(360, 0), Gap(sp8)), func() {
		for _, t := range toasts {
			Container(Attrs(Row, Expand, Corners(radiusM), BackgroundVec(bgRaised), elevateOverlay, Clip), func() {
				bar := Vec4{140, 55, 45, 1}
				fg := txtMain
				if t.Warn {
					bar = Vec4{20, 80, 55, 1}
					fg = txtWarn
				}
				Container(Attrs(FixWidth(sp4), Expand, BackgroundVec(bar)), func() {})
				Container(Attrs(Pad2(sp8, sp12)), func() {
					Label(t.Text, TextColorVec(fg), FontSize(12))
				})
			})
		}
	})
}

// aboutModal shows version and project info.
func (m *model) aboutModal() {
	modal(aboutModalW, func() { m.about = false }, func() {
		Container(Attrs(Gap(sp8)), func() {
			txt("optiscaler-manager " + m.cfg.Version)
			muted("OptiScaler manager for local games — Linux + Steam.")
			muted("GUI: go-shirei (pinned v0.6.6)")
			if focusableButton(SymILeft, "Close") {
				m.about = false
			}
		})
	})
}
