package gui

import (
	"context"
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
			m.toolbar(context.Background())
			// The virtualized views must sit directly inside the expanding
			// column: they size to the remaining space and render nothing
			// inside auto-sized wrappers (upstream demos do the same).
			if m.auditGrid {
				m.auditTable()
			} else if m.state.Mode == ui.ViewList {
				m.actionList()
			} else {
				m.gridView()
			}
			m.statusBar()
		})
		m.toastOverlay()
		if m.about {
			m.aboutModal()
		} else if m.state.Confirm != nil {
			m.confirmModal()
		} else if m.state.Selected != "" {
			m.dashboard()
		}
	})
}

// actionList is the fuzzy-filtered, actionable-first virtualized game list.
func (m *model) actionList() {
	rows := m.visibleRows()
	VirtualListView("games", len(rows),
		func(i int) any { return rows[i].InstallDir },
		func(i int, w float32) float32 { return 30 },
		func(i int, w float32) {
			e := rows[i]
			Container(Attrs(Row, CrossMid, Gap(10), Pad2(3, 12), MinSize(w, 30)), func() {
				Container(Attrs(Row, Gap(4)), func() {
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

// dashboard is the per-game modal with status and actions.
func (m *model) dashboard() {
	e := m.selectedRow()
	if e == nil {
		if m.sess != nil {
			m.sess.Select("")
		}
		return
	}
	Modal(520, func() {
		if m.sess != nil {
			m.sess.Select("")
		}
	}, func() {
		Container(Attrs(Pad(18), Gap(8), BackgroundVec(bgPanel)), func() {
			txt(e.Title)
			muted(e.InstallDir)
			txt("Status: " + statusLabel(e))
			if m.state.Busy != "" {
				muted("Working…")
				return
			}
			if m.sess == nil {
				return
			}
			if Button(SymIRight, quickLabel(e)) {
				m.sess.QuickInstall(e.InstallDir)
			}
			if e.Actionable && Button(SymIRight, "Rollback") {
				m.sess.Rollback(e.InstallDir)
			}
			if e.Status == domain.StatusCommitted && Button(SymIRight, "Open OptiScaler.ini in editor") {
				m.sess.OpenINI(e.InstallDir)
			}
			if Button(SymILeft, "Close") {
				m.sess.Select("")
			}
		})
	})
}

// confirmModal renders the session's pending consent gate.
func (m *model) confirmModal() {
	c := m.state.Confirm
	Modal(460, func() {
		if m.sess != nil {
			m.sess.AnswerConfirm(false)
		}
	}, func() {
		Container(Attrs(Pad(18), Gap(8), BackgroundVec(bgPanel)), func() {
			txt(c.Message)
			if m.sess == nil {
				return
			}
			if Button(SymIRight, "Proceed") {
				m.sess.AnswerConfirm(true)
			}
			if Button(SymILeft, "Cancel") {
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

func viewToggleLabel(mode ui.ViewMode) string {
	if mode == ui.ViewGrid {
		return "View: grid"
	}
	return "View: list"
}
