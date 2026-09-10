package gui

import (
	"strings"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
	"github.com/cr1cr1/optiscaler-manager/internal/version"
)

// dlssSnapshotItem is the restore menu's observability seam: one entry per
// rendered menu row (snapshot id, label, screen rect, container id for
// keyboard-focus tests).
type dlssSnapshotItem struct {
	id    string
	label string
	rect  Rect
	cid   ContainerId
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
	Container(Attrs(Row, Gap(1), Corners(radiusS), BackgroundVec(toneColor(ui.ToneGreen))), func() {
		Container(Attrs(Focusable, Row, CrossMid, Pad2(3, 3), Corners(radiusS)), func() {
			FocusOnClick()
			CycleFocusOnTab()
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
			CycleFocusOnTab()
			m.dlssArrowID = CurrentId()
			m.dlssArrowRect = GetScreenRectOf(CurrentId())
			activated := false
			if HasFocus() && (GetFrameInput().Key == KeyEnter || GetFrameInput().Key == KeySpace) {
				GetFrameInput().Key = KeyCodeNone
				activated = true
			}
			Label("▼", FontSize(10), TextColor(0, 0, 96, 1))
			if PressAction() {
				activated = true
			}
			if activated {
				m.openDLSSRestore(e.InstallDir)
			}
		})
	})
	if m.openDLSSDir == e.InstallDir {
		m.dlssRestoreMenu(e)
	}
}

// openDLSSRestore toggles the per-game restore menu; only one is open at a
// time (mirrors openDropdownDir). The snapshot list is captured ONCE at
// open — reading the backup directory per frame would be wasted I/O.
func (m *model) openDLSSRestore(gameDir string) {
	if m.openDLSSDir == gameDir {
		m.openDLSSDir = ""
		return
	}
	snaps := m.sess.DLSSSnapshots(gameDir)
	if len(snaps) == 0 {
		return
	}
	m.openDLSSDir = gameDir
	m.dlssSnaps = snaps
}

// dlssRestoreMenu lists the complete backed-up NVIDIA sets, newest first;
// picking one (mouse or keyboard) asks the session for confirmation.
func (m *model) dlssRestoreMenu(e *ui.GameRow) {
	if len(m.dlssSnaps) == 0 {
		m.openDLSSDir = ""
		return
	}
	menuID := ContainerId(nil)
	Popup(func() {
		Container(Attrs(MinWidth(220), MaxWidth(360), Corners(radiusS), Pad2(sp4, 0), Gap(2), Clip, BackgroundVec(bgPanel), BorderWidth(1), BorderColorVec(border), elevateOverlay), func() {
			ModAttrs(FloatVec(dropdownPosFor(m.dlssArrowRect)))
			menuID = CurrentId()
			m.dlssMenuID = CurrentId()
			m.dlssSnapshotItems = m.dlssSnapshotItems[:0]
			for _, snap := range m.dlssSnaps {
				snap := snap
				label := snap.Label()
				Container(Attrs(Focusable, Row, Expand, Pad2(sp4, sp8), Corners(2)), func() {
					FocusOnClick()
					CycleFocusOnTab()
					activated := false
					if HasFocus() {
						ModAttrs(func(a *AttrSet) {
							a.BorderWidth = 1
							a.BorderColor = focusBorder
						})
						if GetFrameInput().Key == KeyEnter || GetFrameInput().Key == KeySpace {
							GetFrameInput().Key = KeyCodeNone
							activated = true
						}
					}
					m.dlssSnapshotItems = append(m.dlssSnapshotItems, dlssSnapshotItem{id: snap.ID, label: label, rect: GetScreenRectOf(CurrentId()), cid: CurrentId()})
					Label(label, FontSize(12), TextColorVec(txtMain))
					if PressAction() {
						activated = true
					}
					if activated {
						m.openDLSSDir = ""
						m.dispatchRestoreDLSS(e.InstallDir, snap.ID)
					}
				})
			}
		})
	})
	// Dismissal AFTER the popup rendered (menu.go ordering): Esc closes
	// without dispatch and is consumed so the global handler cannot also
	// close the detail panel; a click outside trigger and menu closes too.
	if GetFrameInput().Key == KeyEscape {
		GetFrameInput().Key = KeyCodeNone
		m.openDLSSDir = ""
	}
	if menuID != nil && !IdIsHovered(m.dlssArrowID) && !IdIsHovered(menuID) && GetFrameInput().Mouse == MouseClick {
		m.openDLSSDir = ""
	}
}

// dropdownPosFor anchors a popup below a screen rect, clamped to the
// window — dropdownPos keyed by rect instead of container id.
func dropdownPosFor(r Rect) Vec2 {
	pos := r.Origin
	pos[1] += r.Size[1] + 4
	self := GetResolvedSize()
	if pos[0]+self[0] > GetHost().WindowSize[0] {
		pos[0] = GetHost().WindowSize[0] - self[0] - 4
	}
	if pos[1]+self[1] > GetHost().WindowSize[1] {
		pos[1] = GetHost().WindowSize[1] - self[1] - 4
	}
	pos[0] = max(0, pos[0])
	pos[1] = max(0, pos[1])
	return pos
}
