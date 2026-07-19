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
	Container(Attrs(Viewport, Background(220, 10, 97, 1)), func() {
		Container(Attrs(Pad(14), Gap(10)), func() {
			Container(Attrs(Row, CrossMid, Gap(10)), func() {
				Label("optiscaler-manager")
				if m.state.Busy != "" {
					Label(m.state.Busy)
				}
				Label(m.state.StatusLine)
			})
			TextInput(&m.filter)
			if m.auditGrid {
				m.auditTable()
			} else {
				m.actionList()
			}
			m.toastStrip()
		})
		if m.state.Confirm != nil {
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
			Container(Attrs(Row, CrossMid, Gap(10), Pad2(3, 6), MinSize(w, 30)), func() {
				badge := ""
				if e.Actionable {
					badge = " [" + string(e.Status) + "]"
				} else if e.Status == domain.StatusCommitted {
					badge = " [installed]"
				}
				if e.EAC {
					badge += " [EAC]"
				}
				Label(e.Title + badge)
				var tech []string
				for _, b := range e.TechBadges {
					tech = append(tech, b.Label)
				}
				if len(tech) > 0 {
					Label(strings.Join(tech, ","))
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
		Container(Attrs(Pad(18), Gap(8)), func() {
			Label(e.Title)
			Label(e.InstallDir)
			Label("Status: " + statusLabel(e))
			if m.state.Busy != "" {
				Label("Working…")
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
		Container(Attrs(Pad(18), Gap(8)), func() {
			Label(c.Message)
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

// toastStrip renders active session toasts under the list.
func (m *model) toastStrip() {
	for _, t := range m.state.Toasts {
		prefix := ""
		if t.Warn {
			prefix = "! "
		}
		Label(prefix + t.Text)
	}
}

// auditTable is the raw, sortable dump behind --audit-grid.
func (m *model) auditTable() {
	Table("audit", 26,
		[]TableColumn[ui.GameRow]{
			{Label: "Name", Render: func(r ui.GameRow) { Label(r.Title) },
				Less: func(a, b ui.GameRow) bool { return a.Title < b.Title }},
			{Label: "AppID", Width: 90, Render: func(r ui.GameRow) { Label(r.AppID) }},
			{Label: "Status", Width: 110, Render: func(r ui.GameRow) { Label(statusLabel(&r)) }},
			{Label: "Path", Render: func(r ui.GameRow) { Label(r.InstallDir) }},
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
