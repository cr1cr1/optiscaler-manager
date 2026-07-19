package gui

import (
	"path/filepath"
	"strings"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// rootView re-declares the whole window each frame (immediate mode).
func (m *model) rootView() {
	Container(Attrs(Viewport, Background(220, 10, 97, 1)), func() {
		Container(Attrs(Pad(14), Gap(10)), func() {
			Container(Attrs(Row, CrossMid, Gap(10)), func() {
				Label("optiscaler-manager")
				Label(m.status)
			})
			TextInput(&m.filter)
			if m.auditGrid {
				m.auditTable()
			} else {
				m.actionList()
			}
		})
		if m.eacPending != "" {
			m.eacModal()
		} else if m.selected != "" {
			m.dashboard()
		}
	})
}

// actionList is the fuzzy-filtered, actionable-first virtualized game list.
func (m *model) actionList() {
	rows := filterRows(m.rows, m.filter)
	VirtualListView("games", len(rows),
		func(i int) any { return rows[i].Game.InstallDir },
		func(i int, w float32) float32 { return 30 },
		func(i int, w float32) {
			e := rows[i]
			Container(Attrs(Row, CrossMid, Gap(10), Pad2(3, 6), MinSize(w, 30)), func() {
				badge := ""
				if actionable(e.Status) {
					badge = " [" + string(e.Status) + "]"
				} else if e.Status == domain.StatusCommitted {
					badge = " [installed]"
				}
				if e.EAC {
					badge += " [EAC]"
				}
				Label(e.Game.Name + badge)
				if len(e.Tech) > 0 {
					Label(strings.Join(e.Tech, ","))
				}
				if PressAction() {
					m.selected = e.Game.InstallDir
				}
			})
		})
}

// dashboard is the per-game modal with status and actions.
func (m *model) dashboard() {
	e := m.selectedEntry()
	if e == nil {
		m.selected = ""
		return
	}
	Modal(520, func() { m.selected = "" }, func() {
		Container(Attrs(Pad(18), Gap(8)), func() {
			Label(e.Game.Name)
			Label(e.Game.InstallDir)
			Label("Status: " + statusText(*e))
			if m.busy {
				Label("Working…")
				return
			}
			if Button(SymIRight, "Install") {
				if decideInstall(*e) == confirmEAC {
					m.eacPending = e.Game.InstallDir
				} else {
					go m.install(e.Game.InstallDir, false)
				}
			}
			if e.Status == domain.StatusCommitted && Button(SymIRight, "Uninstall") {
				go m.uninstall(e.Game.InstallDir)
			}
			if actionable(e.Status) && Button(SymIRight, "Rollback") {
				go m.rollback(e.Game.InstallDir)
			}
			if e.Status == domain.StatusCommitted && e.InjectionDir != "" &&
				Button(SymIRight, "Open OptiScaler.ini in editor") {
				go m.openEditor(filepath.Join(e.InjectionDir, "OptiScaler.ini"))
			}
			if Button(SymILeft, "Close") {
				m.selected = ""
			}
		})
	})
}

// eacModal gates installs into anti-cheat-protected games.
func (m *model) eacModal() {
	Modal(460, func() { m.eacPending = "" }, func() {
		Container(Attrs(Pad(18), Gap(8)), func() {
			Label("This game uses Easy Anti-Cheat.")
			Label("Installing OptiScaler into it may result in a ban.")
			if Button(SymIRight, "Install anyway") {
				dir := m.eacPending
				m.eacPending = ""
				m.selected = ""
				go m.install(dir, true)
			}
			if Button(SymILeft, "Cancel") {
				m.eacPending = ""
			}
		})
	})
}

// auditTable is the raw, sortable dump behind --audit-grid.
func (m *model) auditTable() {
	Table("audit", 26,
		[]TableColumn[app.LibraryEntry]{
			{Label: "Name", Render: func(r app.LibraryEntry) { Label(r.Game.Name) },
				Less: func(a, b app.LibraryEntry) bool { return a.Game.Name < b.Game.Name }},
			{Label: "AppID", Width: 90, Render: func(r app.LibraryEntry) { Label(r.Game.AppID) }},
			{Label: "Tech", Width: 150, Render: func(r app.LibraryEntry) { Label(strings.Join(r.Tech, ",")) }},
			{Label: "Status", Width: 110, Render: func(r app.LibraryEntry) { Label(statusText(r)) }},
			{Label: "Path", Render: func(r app.LibraryEntry) { Label(r.Game.InstallDir) }},
		},
		filterRows(m.rows, m.filter),
		func(r app.LibraryEntry) any { return r.Game.InstallDir },
		0)
}
