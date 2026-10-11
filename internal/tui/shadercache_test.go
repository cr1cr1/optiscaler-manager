package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// The TUI detail screen carries the same shader-cache status as the GUI
// section (issue 078) — read-only lines (actions stay GUI-only for now:
// the TUI has no click targets and new keybindings are a follow-up).
// TestDetailViewShaderStatus: a game with caches names the totals; a
// game without says none found.
func TestDetailViewShaderStatus(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "game.dxvk-cache"), make([]byte, 240), 0o644); err != nil {
		t.Fatal(err)
	}
	row := ui.GameRow{Title: "Cache Game", AppID: "101", InstallDir: dir, Platform: "Steam"}
	plain := sgrRE.ReplaceAllString(detailModelFor(t, row).detailView(100, 40), "")
	if !strings.Contains(plain, "Shader caches:") {
		t.Fatalf("shader status line missing: %q", plain)
	}
	if !strings.Contains(plain, "240 B") {
		t.Errorf("shader totals missing the cache size: %q", plain)
	}

	empty := ui.GameRow{Title: "Clean Game", AppID: "102", InstallDir: t.TempDir(), Platform: "Steam"}
	plainEmpty := sgrRE.ReplaceAllString(detailModelFor(t, empty).detailView(100, 40), "")
	if !strings.Contains(plainEmpty, "Shader caches: none found") {
		t.Errorf("empty game missing the none-found line: %q", plainEmpty)
	}
}
