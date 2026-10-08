package gui

import (
	"testing"

	. "go.hasen.dev/shirei"
)

// Issue 033: the detail pane's action buttons flow horizontally and wrap
// with the pane width; install/uninstall labels name OptiScaler; Launch
// is "Launch Game"; the settings modal's Apply/Close share a row.

func TestDetailActionButtonsWrapHorizontal(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	t.Run("wide pane keeps one line", func(t *testing.T) {
		// Tall window: the 2:3 cover pushes the action row near the fold,
		// and shirei culls clipped Viewport children (zero rect).
		headlessFrames(t, 1600, 1400)
		keyFrame(KeyCodeNone, 0, m.rootView)
		keyFrame(KeyCodeNone, 0, m.rootView)
		for name, r := range map[string]Rect{"install": m.quickBtnRect, "launch": m.launchBtnRect, "open folder": m.openFolderRect} {
			if r.Size[0] == 0 {
				t.Fatalf("%s button not rendered", name)
			}
		}
		if m.quickBtnRect.Origin[1] != m.launchBtnRect.Origin[1] ||
			m.quickBtnRect.Origin[1] != m.openFolderRect.Origin[1] {
			t.Errorf("buttons not on one line in a wide pane: install y %.0f, launch y %.0f, folder y %.0f",
				m.quickBtnRect.Origin[1], m.launchBtnRect.Origin[1], m.openFolderRect.Origin[1])
		}
	})

	t.Run("narrow pane wraps inside the panel", func(t *testing.T) {
		headlessFrames(t, 900, 800)
		keyFrame(KeyCodeNone, 0, m.rootView)
		keyFrame(KeyCodeNone, 0, m.rootView)
		if m.openFolderRect.Origin[1] <= m.quickBtnRect.Origin[1] {
			t.Errorf("open folder (y %.0f) did not wrap below install (y %.0f) in the narrow pane",
				m.openFolderRect.Origin[1], m.quickBtnRect.Origin[1])
		}
		panelRight := m.detailPanelRect.Origin[0] + m.detailPanelRect.Size[0]
		for name, r := range map[string]Rect{"install": m.quickBtnRect, "launch": m.launchBtnRect, "open folder": m.openFolderRect} {
			if right := r.Origin[0] + r.Size[0]; right > panelRight+1 {
				t.Errorf("%s (right edge %.0f) sticks out of the panel (right edge %.0f)", name, right, panelRight)
			}
		}
	})
}

func TestSettingsApplyCloseSameRow(t *testing.T) {
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})
	m.settingsOpen = true

	headlessFrames(t, 1100, 800)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)

	if m.settingsApplyRect.Size[0] == 0 || m.settingsCloseRect.Size[0] == 0 {
		t.Fatalf("settings buttons not captured: apply %+v, close %+v", m.settingsApplyRect, m.settingsCloseRect)
	}
	if m.settingsApplyRect.Origin[1] != m.settingsCloseRect.Origin[1] {
		t.Errorf("Apply (y %.0f) and Close (y %.0f) are not on the same row",
			m.settingsApplyRect.Origin[1], m.settingsCloseRect.Origin[1])
	}
}
