package ui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// containerFixture returns a library root holding two game subdirectories
// (Alpha, Beta), each nesting its exe one level down like T1's
// two-gamey-children fixture — a scan root, not a game itself.
func containerFixture(t *testing.T) (root, alpha, beta string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "Games")
	alpha = filepath.Join(root, "Alpha")
	beta = filepath.Join(root, "Beta")
	writeUIFile(t, filepath.Join(alpha, "bin", "alpha.exe"), "GAME")
	writeUIFile(t, filepath.Join(beta, "bin", "beta.exe"), "GAME")
	return root, alpha, beta
}

func rowDirs(rows []GameRow) map[string]GameRow {
	out := map[string]GameRow{}
	for _, r := range rows {
		out[r.InstallDir] = r
	}
	return out
}

// TestMergeExtraDirs_SkipsContainer: an extra dir that is a container (no
// own exe, several game-bearing children) must not get a self-row — its
// games already surface as individual rows via the recursive scan.
func TestMergeExtraDirs_SkipsContainer(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	root, alpha, beta := containerFixture(t)
	e.sess.deps.Settings.ExtraDirs = []string{root}

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	rows := rowDirs(e.sess.Snapshot().Rows)
	if _, ok := rows[canonicalDir(alpha)]; !ok {
		t.Errorf("child game Alpha missing from rows: %v", rows)
	}
	if _, ok := rows[canonicalDir(beta)]; !ok {
		t.Errorf("child game Beta missing from rows: %v", rows)
	}
	if r, ok := rows[canonicalDir(root)]; ok {
		t.Errorf("container root must not get a self-row, got %+v", r)
	}
}

// TestMergeExtraDirs_GameDirRowKept: a game-like extra dir the recursive
// scan does not surface (its exe sits directly inside it, not in a
// subdirectory) keeps its manual self-row with its title.
func TestMergeExtraDirs_GameDirRowKept(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	game := filepath.Join(t.TempDir(), "SoloGame")
	writeUIFile(t, filepath.Join(game, "solo.exe"), "GAME")
	e.sess.deps.Settings.ExtraDirs = []string{game}

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	r, ok := rowDirs(e.sess.Snapshot().Rows)[canonicalDir(game)]
	if !ok {
		t.Fatal("game-like extra dir lost its self-row")
	}
	if r.Title != "SoloGame" {
		t.Errorf("row title = %q, want %q", r.Title, "SoloGame")
	}
}

// TestScan_StaleContainerRowNotResurrected: a row for a container extra dir
// (e.g. written into games.json before container gating existed) must not
// be kept by the scan's in-flight merge — container install dirs are scan
// roots, and their stale rows are dropped.
func TestScan_StaleContainerRowNotResurrected(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	root, alpha, _ := containerFixture(t)
	e.sess.deps.Settings.ExtraDirs = []string{root}

	// Seed the stale container row as a warm cache from a pre-gating
	// build would have left it.
	e.sess.st.Rows = append(e.sess.st.Rows, GameRow{
		Title:      "Games",
		AppID:      "custom_Games",
		InstallDir: canonicalDir(root),
		Platform:   domain.StoreManual.String(),
		Store:      domain.StoreManual,
	})

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	rows := rowDirs(e.sess.Snapshot().Rows)
	if r, ok := rows[canonicalDir(root)]; ok {
		t.Errorf("stale container row resurrected by scan: %+v", r)
	}
	if _, ok := rows[canonicalDir(alpha)]; !ok {
		t.Errorf("child game Alpha missing from rows: %v", rows)
	}
}
