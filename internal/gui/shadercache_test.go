package gui

import (
	"os"
	"path/filepath"
	"testing"

	. "go.hasen.dev/shirei"
)

// The detail pane hosts the shader-cache section (issue 078): one row
// per discovered cache with Clear where deletion is safe, the env
// presets, and Warm where a watchdog has something to watch. Rows
// render only when present; the section itself always renders.
func TestDetailShaderSection(t *testing.T) {
	sess, gameRoot := guiFakes(t)
	writeGUIFile(t, filepath.Join(gameRoot, "bin", "game.dxvk-cache"), "DXVKDATA")
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame

	if m.shaderClearRect.Size[0] == 0 {
		t.Fatal("shader Clear button not rendered with a DXVK cache present")
	}
	if m.shaderWarmRect.Size[0] == 0 {
		t.Fatal("shader Warm button not rendered with a trackable cache present")
	}
	if m.shaderEnvRect.Size[0] == 0 {
		t.Fatal("shader env preset not rendered")
	}

	// The pane's Clear drops the file through the session op.
	clickRect(m.shaderClearRect, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if _, err := os.Stat(filepath.Join(gameRoot, "bin", "game.dxvk-cache")); !os.IsNotExist(err) {
		t.Fatal("pane Clear did not remove the DXVK cache")
	}

	// The first env preset toggles the DXVK state cache through the
	// existing per-game env pin: unset reads as driver-default on, so
	// the first click turns it off and the second back on.
	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	clickRect(m.shaderEnvRect, m.rootView)
	if got := sess.UmuEnvFor(row.InstallDir)["DXVK_STATE_CACHE"]; got != "0" {
		t.Fatalf("DXVK_STATE_CACHE = %q, want off", got)
	}
	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	clickRect(m.shaderEnvRect, m.rootView)
	if got := sess.UmuEnvFor(row.InstallDir)["DXVK_STATE_CACHE"]; got != "1" {
		t.Fatalf("DXVK_STATE_CACHE = %q, want on after second click", got)
	}
}

// A game with no caches still renders the section shell (header +
// "none found"), but no Clear and no Warm.
func TestDetailShaderSectionEmpty(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)

	if m.shaderClearRect.Size[0] != 0 {
		t.Error("shader Clear rendered with no caches present")
	}
	if m.shaderWarmRect.Size[0] != 0 {
		t.Error("shader Warm rendered with nothing trackable")
	}
	if sess.HasTrackableShaderCache(row.InstallDir) {
		t.Error("empty game reported a trackable cache")
	}
}
