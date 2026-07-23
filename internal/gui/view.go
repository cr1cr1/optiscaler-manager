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
			m.progressBar()
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
				// Panel absent (never rendered this frame): reset the Tab
				// continuation seam so a stale id cannot steer a Tab on the
				// first frame of a reopen into a detached node.
				m.panelFirstID = nil
				m.contentView()
			}
			// Bottom breathing room: rows clip at the list viewport edge,
			// so the last visible row must not sit flush against the bar.
			Element(Attrs(FixHeight(sp8)))
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

// moveListSel moves the keyboard selection by d rows (clamped at both ends)
// and scrolls the selected row into view. Shared by the global key fallback
// and the focused list wrapper so both paths behave identically.
func (m *model) moveListSel(rows []ui.GameRow, d int) {
	m.selIdx += d
	if m.selIdx < 0 {
		m.selIdx = 0
	}
	if n := len(rows); n > 0 && m.selIdx >= n {
		m.selIdx = n - 1
	}
	if len(rows) > 0 {
		VirtualListScrollIntoView("games", rows[m.selIdx].InstallDir)
	}
}

// moveGridSel moves the keyboard cursor by d cards (clamped at both ends)
// and scrolls the cursor card's chunk into view. Shared by the global key
// fallback and the focused grid wrapper so both paths behave identically.
// The grid's virtual list keys items by chunk index (grid.go), so the
// scroll target is the cursor card's chunk: selIdx/cols. In audit mode the
// "grid" list does not exist and the scroll request simply expires.
func (m *model) moveGridSel(rows []ui.GameRow, d int) {
	m.selIdx += d
	if m.selIdx < 0 {
		m.selIdx = 0
	}
	if n := len(rows); n > 0 && m.selIdx >= n {
		m.selIdx = n - 1
	}
	if len(rows) > 0 {
		cols := m.cols
		if cols < 1 {
			cols = 1
		}
		VirtualListScrollIntoView("grid", m.selIdx/cols)
	}
}

// toggleListDetail opens the detail panel for the selected row, or closes
// it when the panel already shows that row (Enter toggles). Shared by the
// global key fallback and the focused list wrapper.
func (m *model) toggleListDetail(rows []ui.GameRow) {
	if len(rows) == 0 || m.sess == nil {
		return
	}
	if dir := rows[m.selIdx].InstallDir; m.state.Selected == dir {
		m.sess.Select("") // toggle: Enter closes the open panel
	} else {
		m.sess.Select(dir)
	}
}

// focusCursorCard hands keyboard focus to the cursor card after a focused
// card's arrow move, so focus and cursor never diverge. The target's fresh
// id is in the registry only when it already rendered this frame (backward
// moves); otherwise — forward moves (the card renders later this frame) or
// a card scrolled away — the deferred cardFocusPending re-assert lands
// focus on the first frame the target card renders.
func (m *model) focusCursorCard() {
	if len(m.gridRows) == 0 {
		return
	}
	dir := m.gridRows[m.selIdx].InstallDir
	if id := m.cardIDs[dir]; id != nil {
		FocusImmediateOn(id)
		return
	}
	m.cardFocusPending = dir
}

// releaseGridInnerFocus drops keyboard focus held INSIDE the grid when an
// arrow key moves the cursor through this global handler: a focused card
// consumes arrows during render, so an arrow reaching here while focus is
// inside the grid means an inner control (button, dropdown trigger) holds
// it — and a cursor move must leave no element of the previous card
// focused (at most one focused element, one ring). The mode gate keeps
// list/audit frames (where gridFocusWithin is stale) untouched.
func (m *model) releaseGridInnerFocus() {
	if m.gridFocusWithin && !m.auditGrid && m.state.Mode != ui.ViewList {
		ClearFocus()
	}
}

// handleGlobalKeys runs at the very end of the frame so focused widgets get
// first pick of the key stream. In grid mode arrows move the card selection
// (±1 across, ±cols up/down); in list mode Up/Down move one row (and scroll
// the row into view) while Left/Right do nothing. Enter toggles the detail
// panel for the selected row (open when closed, closed when already open),
// Escape closes it. Modals own their own keys, so nothing runs while one is
// open.
func (m *model) handleGlobalKeys() {
	if m.sess == nil || m.about || m.settingsOpen || m.state.Confirm != nil {
		return
	}
	// `/` anywhere focuses the search field (unless it is already focused —
	// then the field consumed the character itself).
	if FrameInput.Text == "/" && !m.libraryEmpty() {
		FocusImmediateOn(m.searchID)
		FrameInput.Text = ""
		return
	}
	rows := m.visibleRows()
	if n := len(rows); n > 0 && m.selIdx >= n {
		m.selIdx = n - 1
	}
	// Virtualization boundary: Tab from the last button of the last visible
	// card would wrap to the sidebar (off-screen cards aren't in the
	// focusables registry). Intercept it, scroll forward, and re-assert
	// focus on the newly-visible card.
	if FrameInput.Key == KeyTab && InputState.Modifiers&ModShift == 0 &&
		m.cardLastButtonID != nil && IdHasFocus(m.cardLastButtonID) &&
		!m.auditGrid && m.state.Mode != ui.ViewList &&
		m.selIdx < len(rows)-1 {
		FrameInput.Key = KeyCodeNone
		ClearFocus()
		m.selIdx++
		c := m.cols
		if c < 1 {
			c = 1
		}
		VirtualListScrollIntoView("grid", m.selIdx/c)
		m.cardFocusPending = rows[m.selIdx].InstallDir
		return
	}
	cols := m.cols
	if cols < 1 {
		cols = 1
	}
	listMode := m.state.Mode == ui.ViewList && !m.auditGrid
	switch FrameInput.Key {
	case KeyRight:
		if listMode {
			return
		}
		m.moveGridSel(rows, 1)
		m.releaseGridInnerFocus()
	case KeyLeft:
		if listMode {
			return
		}
		m.moveGridSel(rows, -1)
		m.releaseGridInnerFocus()
	case KeyDown:
		if listMode {
			m.moveListSel(rows, 1)
			FrameInput.Key = KeyCodeNone
			return
		}
		m.moveGridSel(rows, cols)
		m.releaseGridInnerFocus()
	case KeyUp:
		if listMode {
			m.moveListSel(rows, -1)
			FrameInput.Key = KeyCodeNone
			return
		}
		m.moveGridSel(rows, -cols)
		m.releaseGridInnerFocus()
	case KeyEnter:
		m.toggleListDetail(rows)
	case KeyEscape:
		if m.state.Selected != "" {
			m.sess.Select("")
		}
	default:
		return
	}
	if listMode && len(rows) > 0 {
		VirtualListScrollIntoView("games", rows[m.selIdx].InstallDir)
	}
	FrameInput.Key = KeyCodeNone
}

// actionList is the fuzzy-filtered, actionable-first virtualized game list.
// The Focusable wrapper makes the list a Tab stop: while focused it draws
// the focus ring and owns Up/Down/Enter (consumed so the global fallback at
// frame end cannot double-fire); Tab/Shift-Tab always cycle focus away.
func (m *model) actionList() {
	rows := m.visibleRows()
	m.listFocusRing = false
	if len(rows) == 0 {
		m.emptyState()
		return
	}
	// Grow(1)+Expand are required: virtualized lists render nothing inside
	// auto-sized wrappers (see the rootView comment). BorderWidth stays 1 so
	// the focus ring never shifts layout; only the color flips when focused.
	Container(Attrs(Focusable, Grow(1), Expand, BorderWidth(1), BorderColorVec(Vec4{0, 0, 0, 0})), func() {
		m.listID = CurrentId()
		// Row clicks set listFocusPending because opening the detail panel
		// re-nests this wrapper — shirei identities are path-scoped, so focus
		// grabbed mid-click would be orphaned. Re-assert once, on the fresh id.
		if m.listFocusPending {
			m.listFocusPending = false
			FocusImmediateOn(m.listID)
		}
		CycleFocusOnTab()
		FocusOnClick()
		// HasFocus only reports the container currently being built — capture
		// it here, at the wrapper (see the themedInput comment).
		focused := HasFocus()
		if focused {
			m.listFocusRing = true
			ModAttrs(func(a *AttrSet) { a.BorderColor = focusBorder })
			switch FrameInput.Key {
			case KeyDown:
				m.moveListSel(rows, 1)
				FrameInput.Key = KeyCodeNone
			case KeyUp:
				m.moveListSel(rows, -1)
				FrameInput.Key = KeyCodeNone
			case KeyEnter:
				m.toggleListDetail(rows)
				FrameInput.Key = KeyCodeNone
			}
		}
		// Row-rect seams rebuild every frame the list renders: indexes track
		// the current row set, and the selection band clears when nothing is
		// selected.
		m.listRowRects = make([]Rect, len(rows))
		m.listSelectedRect = Rect{}
		VirtualListView("games", len(rows),
			func(i int) any { return rows[i].InstallDir },
			func(i int, w float32) float32 { return 30 },
			func(i int, w float32) {
				e := rows[i]
				Container(Attrs(Row, CrossMid, Gap(sp8), Pad2(3, sp12), MinSize(w, 34), Corners(radiusS)), func() {
					m.listRowRects[i] = GetScreenRectOf(CurrentId())
					if i == m.selIdx {
						m.listSelRect = m.listRowRects[i]
						ModAttrs(func(a *AttrSet) {
							a.BorderWidth = 1.5
							a.BorderColor = accent
						})
					}
					// The session-selected row keeps its selBg band even under
					// the pointer: selection wins over hover so the open game
					// stays visible while the mouse roams.
					if m.state.Selected == e.InstallDir {
						m.listSelectedRect = m.listRowRects[i]
						ModAttrs(BackgroundVec(selBg))
					} else if IsHovered() {
						ModAttrs(BackgroundVec(bgRaised))
					}
					// List rows keep their cover thumbnails: a small portrait
					// tile before the status pills.
					Container(Attrs(FixSize(20, 30), Corners(radiusS), Clip), func() {
						m.coverArt(e, 20, 30)
					})
					Container(Attrs(Row, Gap(sp4)), func() {
						if b, ok := statusPill(&e); ok {
							badgePill(b.Label, b.Tone)
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
						// A row click is also a cursor move and a focus grab:
						// the wrapper's FocusOnClick is hover-timing dependent,
						// so the press sets both deterministically. The pending
						// flag re-asserts focus after the panel re-nests the
						// wrapper (see actionList).
						m.selIdx = i
						FocusImmediateOn(m.listID)
						m.listFocusPending = true
						m.sess.Select(e.InstallDir)
					}
				})
			})
	})
}

// detailPanel is the right-docked per-game panel that replaces the old
// dashboard modal: the grid stays visible beside it. Escape (global keys)
// and the close button both dismiss it.
func (m *model) detailPanel() {
	// Reset whenever the panel is absent (the e == nil early return below)
	// or re-rendering: a stale id must never steer the panel Tab
	// continuation (grid.go) into a detached node on the first frame of a
	// reopen. Re-captured below by the header Close button — the panel's
	// FIRST focusable in render order, which every panel renders, so the
	// continuation works for clean games (no version dropdown) too.
	m.panelFirstID = nil
	e := m.selectedRow()
	if e == nil {
		if m.sess != nil {
			m.sess.Select("")
		}
		return
	}
	panelW := detailPanelWidth(WindowSize[0])
	m.openINIRect = Rect{}
	// Viewport on a Row child absorbs leftover main-axis space, defeating
	// FixWidth — the scrollable column nests inside the fixed-width shell.
	Container(Attrs(FixWidth(panelW), Expand, BackgroundVec(bgPanel)), func() {
		m.detailPanelRect = GetScreenRectOf(CurrentId())
		Container(Attrs(Grow(1), Expand, Pad(sp16), Gap(sp12), Viewport, Clip), func() {
			Container(Attrs(Row, CrossMid, Gap(sp8)), func() {
				Label(e.Title, FontSize(16), TextColorVec(txtMain), FontWeight(WeightBold))
				Filler(1)
				if m.sess != nil && m.panelCloseButton() {
					m.sess.Select("")
				}
				// Shift+Tab on the panel's first focusable (the header Close
				// button) reverses the continuation (grid.go): focus returns
				// to the selected card. The panel renders after the grid, so
				// the card's id in this frame's registry is fresh and
				// resolves directly; a card scrolled out of the virtualized
				// grid re-asserts via the deferred cardFocusPending on its
				// next render instead. Grid mode only — list/audit frames
				// keep the default reverse walk.
				if !m.auditGrid && m.state.Mode != ui.ViewList &&
					m.panelFirstID != nil && IdHasFocus(m.panelFirstID) &&
					FrameInput.Key == KeyTab && InputState.Modifiers&ModShift != 0 {
					FrameInput.Key = KeyCodeNone
					if id := m.cardIDs[m.state.Selected]; id != nil {
						FocusImmediateOn(id)
					} else {
						m.cardFocusPending = m.state.Selected
					}
				}
			})
			coverW := panelW - 2*sp16
			m.coverArt(*e, coverW, coverW*coverRatio)
			muted(e.InstallDir)
			Container(Attrs(Row, Gap(sp4), CrossMid), func() {
				txt("Status:")
				badgePill(statusLabel(e), statusTone(e))
				if e.EAC {
					badgePill("EAC", ui.ToneRed)
				}
				m.protonTierPill(e.ProtonTier)
			})
			if pills := versionPills(e); len(pills) > 0 {
				Container(Attrs(Row, Gap(sp4)), func() {
					start := 0
					// The OptiScaler pill is the version dropdown; component
					// and Proton pills stay static.
					if b, ok := optiBadge(e); ok {
						m.versionDropdown(e, b.Label, b.Tone)
						start = 1
					}
					for _, p := range pills[start:] {
						badgePill(p.Label, p.Tone)
					}
				})
			}
			Container(Attrs(Gap(2)), func() {
				if e.Platform != "" {
					detailField("Platform", e.Platform)
				}
				if e.AppID != "" {
					detailField("AppID", e.AppID)
				}
				if e.SteamAppID != "" {
					detailField("Steam AppID", e.SteamAppID)
				}
			})
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
			if e.CanOpenINI() {
				Container(Attrs(Row), func() {
					m.openINIRect = GetScreenRectOf(CurrentId())
					if focusableButton(SymIRight, "Open OptiScaler.ini in editor") {
						m.sess.OpenINI(e.InstallDir)
					}
				})
			}
			scrollBars()
		})
	})
}

// panelCloseButton renders the detail panel's header Close button as a
// focusable control (focusableButton's pattern) and captures its container
// id in m.panelFirstID: the Close button is the panel's FIRST focusable in
// render order — rendered before the version pills/dropdown — so the panel
// Tab continuation (grid.go) jumps here, for clean games with no version
// dropdown too. focusableButton cannot serve here: it does not expose its
// wrapper's container id, which the continuation seam needs.
func (m *model) panelCloseButton() bool {
	activated := false
	Container(Attrs(Focusable, Corners(6)), func() {
		CycleFocusOnTab()
		FocusOnClick()
		m.panelFirstID = CurrentId()
		if HasFocus() {
			ModAttrs(func(a *AttrSet) {
				a.BorderWidth = 2
				a.BorderColor = focusBorder
			})
			if FrameInput.Key == KeyEnter || FrameInput.Key == KeySpace {
				FrameInput.Key = KeyCodeNone
				activated = true
			}
		}
		if ButtonExt("Close", ButtonAttrs{Icon: TypCancel}) {
			activated = true
		}
	})
	return activated
}

// detailField is one label/value metadata row in the detail panel.
func detailField(label, value string) {
	Container(Attrs(Row, Gap(sp8)), func() {
		Label(label, FontSize(12), TextColorVec(txtMuted))
		Filler(1)
		Label(value, FontSize(12), TextColorVec(txtMain))
	})
}

// statusTone colors the detail panel's status pill: committed green,
// actionable red, external (on-disk but unmanaged) blue, the rest neutral.
func statusTone(e *ui.GameRow) ui.Tone {
	switch {
	case e.Actionable:
		return ui.ToneRed
	case e.Status == domain.StatusCommitted:
		return ui.ToneGreen
	case e.Status == domain.StatusExternal:
		return ui.ToneBlue
	default:
		return ui.ToneGray
	}
}

// statusPill is the alert-style status badge shared by the list rows and the
// grid card chrome: actionable rows flag their failed/pending state red,
// external installs get the blue status pill. ok=false for rows with nothing
// to flag (the list view draws its own committed marker instead).
func statusPill(e *ui.GameRow) (ui.Badge, bool) {
	switch {
	case e.Actionable:
		return ui.Badge{Label: string(e.Status), Tone: ui.ToneRed}, true
	case e.Status == domain.StatusExternal:
		return ui.Badge{Label: statusLabel(e), Tone: statusTone(e)}, true
	}
	return ui.Badge{}, false
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
	switch e.Status {
	case domain.StatusCommitted:
		return "Uninstall"
	case domain.StatusExternal:
		return "Adopt"
	}
	return "Install"
}
