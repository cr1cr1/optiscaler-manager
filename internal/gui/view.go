package gui

import (
	"strings"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// rootView re-declares the whole window each frame (immediate mode).
func (m *model) rootView() {
	m.drain()
	m.syncFilter()
	Container(Attrs(Viewport, Row, BackgroundVec(bgApp)), func() {
		m.sidebar()
		Container(Attrs(Grow(1), Expand, Gap(0)), func() {
			m.toolbar()
			// The virtualized views must sit directly inside the expanding
			// column: they size to the remaining space and render nothing
			// inside auto-sized wrappers (upstream demos do the same). With
			// the detail panel open the column nests inside a Row that keeps
			// it expanding beside the fixed-width panel.
			if m.state.Selected != "" && !m.auditGrid {
				Container(Attrs(Row, Grow(1), Expand), func() {
					Container(Attrs(Grow(1), Expand), m.contentView)
					m.detailPanel()
				})
			} else {
				m.contentView()
			}
			m.statusBar()
		})
		m.toastOverlay()
		if m.about {
			m.aboutModal()
		} else if m.settingsOpen {
			m.settingsModal()
		} else if m.state.Confirm != nil {
			m.confirmModal()
		}
		m.handleGlobalKeys()
	})
}

// contentView is the active library view (grid, list, or audit table).
func (m *model) contentView() {
	if m.auditGrid {
		m.auditTable()
	} else if m.state.Mode == ui.ViewList {
		m.actionList()
	} else {
		m.gridView()
	}
}

// handleGlobalKeys runs at the very end of the frame so focused widgets get
// first pick of the key stream: arrows move the card selection (±1 across,
// ±cols up/down), Enter opens the detail view, Escape closes it. Modals own
// their own keys, so nothing runs while one is open.
func (m *model) handleGlobalKeys() {
	if m.sess == nil || m.about || m.settingsOpen || m.state.Confirm != nil {
		return
	}
	rows := m.visibleRows()
	if n := len(rows); n > 0 && m.selIdx >= n {
		m.selIdx = n - 1
	}
	cols := m.cols
	if cols < 1 {
		cols = 1
	}
	move := func(d int) {
		m.selIdx += d
		if m.selIdx < 0 {
			m.selIdx = 0
		}
		if n := len(rows); n > 0 && m.selIdx >= n {
			m.selIdx = n - 1
		}
	}
	switch FrameInput.Key {
	case KeyRight:
		move(1)
	case KeyLeft:
		move(-1)
	case KeyDown:
		move(cols)
	case KeyUp:
		move(-cols)
	case KeyEnter:
		if len(rows) > 0 {
			m.sess.Select(rows[m.selIdx].InstallDir)
		}
	case KeyEscape:
		if m.state.Selected != "" {
			m.sess.Select("")
		}
	default:
		return
	}
	FrameInput.Key = KeyCodeNone
}

// actionList is the fuzzy-filtered, actionable-first virtualized game list.
func (m *model) actionList() {
	rows := m.visibleRows()
	if len(rows) == 0 {
		m.emptyState()
		return
	}
	VirtualListView("games", len(rows),
		func(i int) any { return rows[i].InstallDir },
		func(i int, w float32) float32 { return 30 },
		func(i int, w float32) {
			e := rows[i]
			Container(Attrs(Row, CrossMid, Gap(sp12), Pad2(3, sp12), MinSize(w, 30), Corners(radiusS)), func() {
				if IsHovered() {
					ModAttrs(BackgroundVec(bgRaised))
				}
				Container(Attrs(Row, Gap(sp4)), func() {
					if e.Actionable {
						badgePill(string(e.Status), ui.ToneRed)
					} else if e.Status == domain.StatusCommitted {
						badgePill("✦ OptiScaler", ui.TonePurple)
					}
					if e.EAC {
						badgePill("EAC", ui.ToneRed)
					}
				})
				txt(e.Title)
				var tech []string
				for _, b := range e.TechBadges {
					tech = append(tech, b.Label)
				}
				if len(tech) > 0 {
					muted(strings.Join(tech, ","))
				}
				if PressAction() && m.sess != nil {
					m.sess.Select(e.InstallDir)
				}
			})
		})
}

// detailPanel is the right-docked per-game panel that replaces the old
// dashboard modal: the grid stays visible beside it. Escape (global keys)
// and the close button both dismiss it.
func (m *model) detailPanel() {
	e := m.selectedRow()
	if e == nil {
		if m.sess != nil {
			m.sess.Select("")
		}
		return
	}
	Container(Attrs(FixWidth(detailPanelW), Expand, BackgroundVec(bgPanel), Pad(sp16), Gap(sp12), Viewport, Clip), func() {
		Container(Attrs(Row, CrossMid, Gap(sp8)), func() {
			Label(e.Title, FontSize(16), TextColorVec(txtMain), FontWeight(WeightBold))
			Filler(1)
			if m.sess != nil && focusableButton(TypCancel, "Close") {
				m.sess.Select("")
			}
		})
		m.coverArt(*e, 160, 160*coverRatio)
		muted(e.InstallDir)
		Container(Attrs(Row, Gap(sp4), CrossMid), func() {
			txt("Status:")
			badgePill(statusLabel(e), statusTone(e))
		})
		if pills := versionPills(e); len(pills) > 0 {
			Container(Attrs(Row, Gap(sp4)), func() {
				for _, p := range pills {
					badgePill(p.Label, p.Tone)
				}
			})
		}
		if m.sess == nil {
			return
		}
		if m.sess.OpBusy(e.InstallDir) {
			Container(Attrs(Row, Gap(sp8), CrossMid), func() {
				spinnerGlyph()
				muted("Working…")
			})
			if focusableButton(SymILeft, "Cancel") {
				m.sess.CancelOp(e.InstallDir)
			}
			return
		}
		if focusableButton(SymIRight, quickLabel(e)) {
			m.sess.QuickInstall(e.InstallDir)
		}
		if launchable(e) && focusableButton(SymPlay, "Launch") {
			m.launchGame(*e)
		}
		if e.Actionable && focusableButton(SymUndo, "Rollback") {
			m.sess.Rollback(e.InstallDir)
		}
		if e.Status == domain.StatusCommitted && focusableButton(SymIRight, "Open OptiScaler.ini in editor") {
			m.sess.OpenINI(e.InstallDir)
		}
		ScrollBars()
	})
}

// statusTone colors the detail panel's status pill: committed green,
// actionable red, everything else neutral.
func statusTone(e *ui.GameRow) ui.Tone {
	switch {
	case e.Actionable:
		return ui.ToneRed
	case e.Status == domain.StatusCommitted:
		return ui.ToneGreen
	default:
		return ui.ToneGray
	}
}

// confirmModal renders the session's pending consent gate.
func (m *model) confirmModal() {
	c := m.state.Confirm
	modal(confirmModalW, func() {
		if m.sess != nil {
			m.sess.AnswerConfirm(false)
		}
	}, func() {
		Container(Attrs(Gap(sp8), BackgroundVec(bgPanel)), func() {
			txt(c.Message)
			if m.sess == nil {
				return
			}
			if focusableButton(SymIRight, "Proceed") {
				m.sess.AnswerConfirm(true)
			}
			if focusableButton(SymILeft, "Cancel") {
				m.sess.AnswerConfirm(false)
			}
		})
	})
}

// auditTable is the raw, sortable dump behind --audit-grid.
func (m *model) auditTable() {
	Table("audit", 26,
		[]TableColumn[ui.GameRow]{
			{Label: "Name", Render: func(r ui.GameRow) { txt(r.Title) },
				Less: func(a, b ui.GameRow) bool { return a.Title < b.Title }},
			{Label: "AppID", Width: 90, Render: func(r ui.GameRow) { txt(r.AppID) }},
			{Label: "Status", Width: 110, Render: func(r ui.GameRow) { txt(statusLabel(&r)) }},
			{Label: "Path", Render: func(r ui.GameRow) { muted(r.InstallDir) }},
		},
		m.visibleRows(),
		func(r ui.GameRow) any { return r.InstallDir },
		0)
}

// emptyStateCopy is the single guidance line shown when the library (or the
// current filter) has nothing to display.
func emptyStateCopy(query string) string {
	if query != "" {
		return "No games match \"" + query + "\" — clear the search to see the library"
	}
	return "No games found — use Add Game to register a folder"
}

// emptyState renders a centered icon, heading, guidance, and calls to action
// in place of an empty library view: scan/add-directory when the library is
// empty, clear-search when the filter matched nothing.
func (m *model) emptyState() {
	query := m.state.Query
	icon, heading := TypFolderOpen, "No games yet"
	if query != "" {
		icon, heading = SymSearch, "No matches"
	}
	Container(Attrs(Grow(1), Expand, Center, Gap(sp12), Pad(sp24)), func() {
		Container(Attrs(Center, Gap(sp8)), func() {
			Container(Attrs(Row), func() {
				Filler(1)
				Icon(icon, FontSize(36), TextColorVec(txtMuted))
				Filler(1)
			})
			Label(heading, FontSize(18), TextColorVec(txtMain), FontWeight(WeightBold))
			muted(emptyStateCopy(query))
			if m.sess != nil {
				Container(Attrs(Row, Gap(sp8)), func() {
					if query != "" {
						if focusableButton(SymILeft, "Clear search") {
							m.filter = ""
							m.sess.SetQuery("")
						}
						return
					}
					if focusableButton(SymRefresh, "Scan") {
						m.sess.Scan(m.ctx)
					}
					if focusableButton(SymIPlus, "Add directory…") {
						m.sess.PickAndAddDirectory(m.ctx)
					}
				})
			}
		})
	})
}

func statusLabel(e *ui.GameRow) string {
	if e.Status == "" {
		return "not installed"
	}
	return string(e.Status)
}

// quickLabel is the toggle caption matching the reference client.
func quickLabel(e *ui.GameRow) string {
	if e.Status == domain.StatusCommitted {
		return "Uninstall"
	}
	return "Install"
}
