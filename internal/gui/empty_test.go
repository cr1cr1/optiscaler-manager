package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestEmptyStateCopyShown: an empty library must render grandma-proof
// guidance that says how to add games; an empty filter result must say the
// filter matched nothing (distinct copy, no dead end).
func TestEmptyStateCopyShown(t *testing.T) {
	guidance := emptyStateCopy("")
	if !strings.Contains(strings.ToLower(guidance), "add game") {
		t.Errorf("empty-library copy %q does not mention adding games", guidance)
	}
	filtered := emptyStateCopy("cyberpunk")
	if filtered == guidance {
		t.Error("filter-empty copy identical to library-empty copy")
	}
	if !strings.Contains(strings.ToLower(filtered), "match") {
		t.Errorf("filter-empty copy %q does not explain the filter matched nothing", filtered)
	}
	t.Logf("empty-library copy: %q", guidance)
	t.Logf("filter-empty copy: %q", filtered)

	// Both main views render a valid frame with zero rows (grid and list).
	for name, mode := range map[string]ui.ViewMode{"grid": ui.ViewGrid, "list": ui.ViewList} {
		m := newModel(Config{})
		m.state = ui.State{StatusLine: "0 games", Mode: mode}
		out := filepath.Join(t.TempDir(), "empty-"+name+".png")
		if err := renderToPNG(out, 800, 600, m.rootView); err != nil {
			t.Fatalf("renderToPNG empty %s: %v", name, err)
		}
		st, err := os.Stat(out)
		if err != nil || st.Size() == 0 {
			t.Fatalf("empty %s frame: %v", name, err)
		}
		t.Logf("empty %s frame: %d bytes", name, st.Size())
	}
}
