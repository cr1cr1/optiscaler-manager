package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestGUIStartCachedShowsCachedRows: a warm boot (games.json cache present)
// must render the cached rows instantly with the "(cached)" status and never
// enter the scanning busy state — Session.Start, not Scan, is the boot path.
func TestGUIStartCachedShowsCachedRows(t *testing.T) {
	root := t.TempDir()
	sess, _ := guiFakes(t, func(d *ui.Deps) {
		d.SettingsRoot = root
		d.Settings = settings.Defaults()
	})
	cacheJSON := `{"version":3,"rows":[{"Title":"Cached Game","InstallDir":"/games/cached","Platform":"Manual"}]}`
	if err := os.WriteFile(filepath.Join(root, "games.json"), []byte(cacheJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newModel(Config{Session: sess})
	m.boot(context.Background())
	m.drain()

	rows := sess.VisibleRows()
	if len(rows) != 1 || rows[0].Title != "Cached Game" {
		t.Fatalf("cached boot rows %+v, want the single cached row", rows)
	}
	if got, want := m.state.StatusLine, "1 games (cached)"; got != want {
		t.Errorf("status line %q, want %q", got, want)
	}
	if m.state.Busy == "Scanning…" {
		t.Error("cached boot must not enter the scanning busy state")
	}
	t.Logf("cached boot: %q, busy %q", m.state.StatusLine, m.state.Busy)
}
