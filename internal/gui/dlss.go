package gui

import (
	"strings"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
	"github.com/cr1cr1/optiscaler-manager/internal/version"
)

// dlssSnapshotItem is the restore menu's observability seam: one entry per
// rendered menu row (snapshot id, label, screen rect, and the keyboard/
// hover highlight state).
type dlssSnapshotItem struct {
	id    string
	label string
	rect  Rect
	hl    bool
}

// componentPill renders one version-pill entry: the DLSS pill is the
// interactive update/restore control, every other component a static
// badge. Both the card and the detail panel route through this so the
// dispatch rule lives in exactly one place.
func (m *model) componentPill(e *ui.GameRow, p ui.Badge) {
	if isDLSSPill(p.Label) {
		m.dlssControl(e, p.Label)
		return
	}
	badgePill(p.Label, p.Tone)
}

// isDLSSPill reports whether a component label is the DLSS control's pill:
// the versioned "DLSS 4.5" or the bare "DLSS" a version-stripped DLL
// degrades to. "DLSS-FG" is a static badge — frame generation has no
// update path.
func isDLSSPill(label string) bool {
	return label == "DLSS" || strings.HasPrefix(label, "DLSS ")
}

// dlssUpdateTarget is the update-available marker appended to the DLSS
// control's version segment: the best known candidate — the startup
// check's published version, or the download cache when the online half
// is unknown (offline mode, failed lookup), whichever is newer — when the
// applied raw version is either unreadable or older than it. "" = up to
// date (or status unknown).
func dlssUpdateTarget(e *ui.GameRow, online, cached string) string {
	candidate := online
	if cached != "" && (candidate == "" || version.Compare(cached, candidate) > 0) {
		candidate = cached
	}
	if candidate == "" {
		return ""
	}
	if e.DLSSVersion != "" && version.Compare(e.DLSSVersion, candidate) >= 0 {
		return ""
	}
	return candidate
}

// dlssControl renders the DLSS component pill as a dual-color control:
// "DLSS:" keeps the component green, the version renders in main text
// color. Pressing the version area starts the three-DLL NVIDIA update; the
// small arrow opens the backup-set restore menu. Non-DLSS pills and busy
// games fall back to the static badgePill.
func (m *model) dlssControl(e *ui.GameRow, label string) {
	if m.sess == nil || !e.DLSSReady || !isDLSSPill(label) || m.sess.OpBusy(e.InstallDir) {
		if m.openDLSSDir == e.InstallDir {
			m.openDLSSDir = "" // the control is gone; never leave a ghost menu
		}
		badgePill(label, ui.ToneGreen)
		return
	}
	version := strings.TrimSpace(strings.TrimPrefix(label, "DLSS"))
	if target := dlssUpdateTarget(e, m.state.DLSSLatest.Version, m.state.DLSSCached); target != "" {
		if version != "" {
			version += " → "
		}
		version += target
	}
	// The restore menu shares the version dropdown's machinery and its
	// single-open coordination: st.open is this instance's render flag,
	// m.openDLSSDir names the one open menu. A card re-rendering every frame
	// (or the same game's panel instance) whose menu is not the owner clears
	// its flag instead of floating a stale popup.
	st := Use[dropdownState]("dlss-restore")
	if st.open && m.openDLSSDir != e.InstallDir {
		st.open = false
	}
	enterPick := false
	Container(Attrs(Row, Gap(1), Corners(radiusS), BackgroundVec(toneColor(ui.ToneGreen))), func() {
		Container(Attrs(Focusable, Row, CrossMid, Pad2(3, 3), Corners(radiusS)), func() {
			FocusOnClick()
			m.dlssUpdateID = CurrentId()
			m.dlssUpdateRect = GetScreenRectOf(CurrentId())
			activated := false
			if HasFocus() && (GetFrameInput().Key == KeyEnter || GetFrameInput().Key == KeySpace) {
				GetFrameInput().Key = KeyCodeNone
				activated = true
			}
			Label("DLSS:", FontSize(11), TextColor(0, 0, 96, 1))
			Label(version, FontSize(11), TextColorVec(txtMain))
			if PressAction() {
				activated = true
			}
			if activated {
				m.dispatchUpdateDLSS(e.InstallDir)
			}
		})
		Container(Attrs(Focusable, Pad2(3, 5), Corners(radiusS)), func() {
			FocusOnClick()
			m.dlssArrowID = CurrentId()
			m.dlssArrowRect = GetScreenRectOf(CurrentId())
			st.btnID = CurrentId()
			activated := false
			if HasFocus() {
				if st.open {
					// The focused arrow owns the open menu's navigation
					// (the shared dropdown model): Up/Down move the
					// highlight, Enter picks, Space toggles closed.
					var spaceToggle bool
					enterPick, spaceToggle = dropdownMenuKeys(st, len(m.dlssSnaps))
					if spaceToggle {
						activated = true
					}
				}
				if GetFrameInput().Key == KeyEnter || GetFrameInput().Key == KeySpace {
					GetFrameInput().Key = KeyCodeNone
					activated = true
				}
			}
			dropdownArrow()
			if PressAction() {
				activated = true
			}
			if activated {
				st.open = m.openDLSSRestore(e.InstallDir)
				if st.open {
					st.hl = -1 // re-initialize the highlight on the popup frame
				}
			}
		})
	})
	if st.open {
		m.dlssRestoreMenu(e, enterPick)
	}
}

// openDLSSRestore toggles the per-game restore menu and reports the new
// open state; only one is open at a time (mirrors openDropdownDir). The
// snapshot list is captured ONCE at open — reading the backup directory
// per frame would be wasted I/O.
func (m *model) openDLSSRestore(gameDir string) bool {
	if m.openDLSSDir == gameDir {
		m.openDLSSDir = ""
		return false
	}
	snaps := m.sess.DLSSSnapshots(gameDir)
	if len(snaps) == 0 {
		return false
	}
	m.openDLSSDir = gameDir
	m.dlssSnaps = snaps
	return true
}

// dlssRestoreMenu lists the complete backed-up NVIDIA sets, newest first,
// on the shared dropdown machinery (same popup, same keyboard navigation
// and dismissal as the version picker — the arrow trigger keeps focus and
// drives the menu); picking one (mouse or highlighted-Enter) asks the
// session for confirmation.
func (m *model) dlssRestoreMenu(e *ui.GameRow, enterPick bool) {
	st := Use[dropdownState]("dlss-restore")
	if len(m.dlssSnaps) == 0 {
		st.open = false
		m.openDLSSDir = ""
		return
	}
	dropdownPopup(st, 220, 360, func(mouseMoved bool) {
		if st.hl < 0 {
			// Open-time init: the highlight starts on the first (newest) row.
			st.hl = 0
		}
		m.dlssSnapshotItems = m.dlssSnapshotItems[:0]
		for i, snap := range m.dlssSnaps {
			snap := snap
			label := snap.Label()
			dropdownRow(st, i, enterPick, mouseMoved,
				func(hl bool) {
					Label(label, FontSize(12), TextColorVec(txtMain))
					m.dlssSnapshotItems = append(m.dlssSnapshotItems, dlssSnapshotItem{id: snap.ID, label: label, rect: GetScreenRectOf(CurrentId()), hl: hl})
				},
				func(keyboard bool) {
					m.openDLSSDir = ""
					m.dispatchRestoreDLSS(e.InstallDir, snap.ID)
					if keyboard {
						FocusImmediateOn(st.btnID)
					}
				})
		}
	})
	// The card's click handler excludes presses under the open menu; the
	// popup's panel id lives in st.menuID (dropdownPopup), mirrored here.
	m.dlssMenuID = st.menuID
	// Dismissal AFTER the popup rendered (menu.go ordering): Esc closes
	// without dispatch and is consumed so the global handler cannot also
	// close the detail panel; a click outside trigger and menu closes too.
	dropdownDismiss(st, func() { m.openDLSSDir = "" })
}
