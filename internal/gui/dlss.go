package gui

import (
	"strings"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// dlssSnapshotItem is the restore menu's observability seam: one entry per
// rendered menu row (snapshot id, label, screen rect).
type dlssSnapshotItem struct {
	id    string
	label string
	rect  Rect
}

// dlssControl renders the DLSS component pill as a dual-color control:
// "DLSS:" keeps the component green, the version renders in main text
// color. Pressing the version area starts the three-DLL NVIDIA update; the
// small arrow opens the backup-set restore menu. Non-DLSS pills and busy
// games fall back to the static badgePill.
func (m *model) dlssControl(e *ui.GameRow, label string) {
	if m.sess == nil || !strings.HasPrefix(label, "DLSS ") || m.sess.OpBusy(e.InstallDir) {
		if m.openDLSSDir == e.InstallDir {
			m.openDLSSDir = "" // the control is gone; never leave a ghost menu
		}
		badgePill(label, ui.ToneGreen)
		return
	}
	version := strings.TrimSpace(strings.TrimPrefix(label, "DLSS"))
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
// time (mirrors openDropdownDir).
func (m *model) openDLSSRestore(gameDir string) {
	if m.openDLSSDir == gameDir {
		m.openDLSSDir = ""
		return
	}
	m.openDLSSDir = gameDir
}

// dlssRestoreMenu lists the complete backed-up NVIDIA sets, newest first;
// picking one asks the session for confirmation.
func (m *model) dlssRestoreMenu(e *ui.GameRow) {
	snaps := m.sess.DLSSSnapshots(e.InstallDir)
	if len(snaps) == 0 {
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
			for _, snap := range snaps {
				snap := snap
				label := snapshotLabel(snap)
				Container(Attrs(Row, Expand, Pad2(sp4, sp8), Corners(2)), func() {
					m.dlssSnapshotItems = append(m.dlssSnapshotItems, dlssSnapshotItem{id: snap.ID, label: label, rect: GetScreenRectOf(CurrentId())})
					Label(label, FontSize(12), TextColorVec(txtMain))
					if PressAction() {
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

func snapshotLabel(s dlss.Snapshot) string {
	version := "unknown"
	for _, f := range s.Files {
		if f.Name == "nvngx_dlss.dll" {
			version = f.Version
			break
		}
	}
	if version == "" {
		version = "unknown"
	}
	return "DLSS " + version + " · " + s.CreatedAt.Local().Format("2006-01-02 15:04")
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
