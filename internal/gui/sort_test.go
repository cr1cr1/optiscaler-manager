package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestGUISortControlChangesOrder: the sort control drives Session.SetSort —
// SortName orders alphabetically, SortDefault restores actionable-first.
func TestGUISortControlChangesOrder(t *testing.T) {
	var st *store.Store
	sess, gameRoot := guiFakes(t, func(d *ui.Deps) { st = d.Store })
	m := newModel(Config{Session: sess})

	// Game One gets a failed (actionable) manifest; "Alpha" sorts before it
	// by name but must not outrank an actionable row under the default sort.
	injDir, err := discovery.ResolveInstallDir(gameRoot)
	if err != nil {
		t.Fatalf("resolve injection dir: %v", err)
	}
	if err := st.Save(&domain.Manifest{
		ID:         domain.ManifestID(injDir),
		Status:     domain.StatusFailed,
		InstallDir: injDir,
		GameRoot:   gameRoot,
	}); err != nil {
		t.Fatalf("save failed manifest: %v", err)
	}
	alphaDir := filepath.Join(t.TempDir(), "Alpha")
	if err := os.MkdirAll(alphaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A real exe makes the dir a game (v0.7): empty dirs are refused.
	if err := os.WriteFile(filepath.Join(alphaDir, "game.exe"), []byte("GAME"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess.Scan(context.Background())
	deadline := time.Now().Add(15 * time.Second)
	for len(sess.VisibleRows()) < 1 && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	sess.AddDirectory(alphaDir)

	rows := sess.VisibleRows()
	if len(rows) != 2 {
		t.Fatalf("rows %v, want 2 (Game One + Alpha)", titles(rows))
	}
	if rows[0].Title != "Game One" || !rows[0].Actionable {
		t.Fatalf("default sort first row %+v, want actionable Game One", rows[0])
	}

	m.setSort(ui.SortName)
	rows = sess.VisibleRows()
	if rows[0].Title != "Alpha" {
		t.Errorf("name sort first row %q, want Alpha", rows[0].Title)
	}

	m.setSort(ui.SortDefault)
	rows = sess.VisibleRows()
	if rows[0].Title != "Game One" {
		t.Errorf("default sort restored first row %q, want actionable Game One", rows[0].Title)
	}
	t.Logf("name sort: %v; default sort: %v", titles(sess.VisibleRows()), titles(rows))
}

func titles(rows []ui.GameRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Title)
	}
	return out
}
