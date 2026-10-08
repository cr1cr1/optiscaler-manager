package gui

import (
	"testing"

	. "go.hasen.dev/shirei"
)

// Issue 033: install/uninstall labels name OptiScaler; Launch is
// "Launch Game"; the settings modal's Apply/Close share a row.
//
// Issue 036: the detail pane groups its action buttons into two rows —
// row 1 the game actions (Launch Game, then Game directory), row 2
// the OptiScaler actions (install/uninstall, rollback, …). Each row
// wraps within itself on narrow panes but the groups never interleave.

func TestDetailActionRowsGrouped(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	rendered := func(t *testing.T) {
		t.Helper()
		for name, r := range map[string]Rect{"install": m.quickBtnRect, "launch": m.launchBtnRect, "open dir": m.openFolderRect} {
			if r.Size[0] == 0 {
				t.Fatalf("%s button not rendered", name)
			}
		}
	}

	t.Run("wide pane: game row above optiscaler row", func(t *testing.T) {
		// Tall window: the 2:3 cover pushes the action rows near the fold,
		// and shirei culls clipped Viewport children (zero rect).
		headlessFrames(t, 1600, 1400)
		keyFrame(KeyCodeNone, 0, m.rootView)
		keyFrame(KeyCodeNone, 0, m.rootView)
		rendered(t)
		launch, openDir, install := m.launchBtnRect, m.openFolderRect, m.quickBtnRect
		if launch.Origin[1] != openDir.Origin[1] {
			t.Errorf("Launch Game (y %.0f) and Game directory (y %.0f) must share row 1",
				launch.Origin[1], openDir.Origin[1])
		}
		if launch.Origin[0] >= openDir.Origin[0] {
			t.Errorf("Launch Game (x %.0f) must sit left of Game directory (x %.0f)",
				launch.Origin[0], openDir.Origin[0])
		}
		if install.Origin[1] <= launch.Origin[1] {
			t.Errorf("install (y %.0f) must sit on a row below the game row (y %.0f)",
				install.Origin[1], launch.Origin[1])
		}
	})

	t.Run("narrow pane: rows wrap but never interleave", func(t *testing.T) {
		// Tall window: two action rows sit below the 2:3 cover, and shirei
		// culls Viewport children clipped past the fold (zero rect).
		headlessFrames(t, 900, 1400)
		keyFrame(KeyCodeNone, 0, m.rootView)
		keyFrame(KeyCodeNone, 0, m.rootView)
		rendered(t)
		launch, openDir, install := m.launchBtnRect, m.openFolderRect, m.quickBtnRect
		if openDir.Origin[1] < launch.Origin[1] {
			t.Errorf("Game directory (y %.0f) wrapped above Launch Game (y %.0f)",
				openDir.Origin[1], launch.Origin[1])
		}
		if install.Origin[1] <= openDir.Origin[1] {
			t.Errorf("install (y %.0f) must stay below the game row (open dir y %.0f) even when wrapped",
				install.Origin[1], openDir.Origin[1])
		}
		panelRight := m.detailPanelRect.Origin[0] + m.detailPanelRect.Size[0]
		for name, r := range map[string]Rect{"install": install, "launch": launch, "open dir": openDir} {
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
