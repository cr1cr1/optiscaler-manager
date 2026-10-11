package tui

import (
	"context"
	"strings"
	"testing"
)

// TestDetailViewRendersGameDirTruth: the TUI detail view renders the
// directory-verified truth line from the session snapshot (issue 079) —
// verified right after install, naming the drift after a foreign edit
// while the manifest still claims committed.
func TestDetailViewRendersGameDirTruth(t *testing.T) {
	e := newTestEnv(t, nil)
	e.sess.Scan(context.Background())
	pollUntil(t, "row to scan", func() bool {
		return findRow(e.sess.Snapshot().Rows, e.gameRoot) != nil
	})
	e.sess.QuickInstall(e.gameRoot)
	pollUntil(t, "row to commit", func() bool {
		if r := findRow(e.sess.Snapshot().Rows, e.gameRoot); r != nil {
			return r.Status == "committed"
		}
		return false
	})

	e.sess.Select(e.gameRoot)
	pollUntil(t, "detail truth to settle", func() bool {
		return e.sess.Snapshot().DetailTruth != nil
	})

	m := New(e.sess, "v0.0.0-test")
	m.detailDir = e.gameRoot
	if out := m.detailView(100, 30); !strings.Contains(out, "Game dir:") || !strings.Contains(out, "verified") {
		t.Fatalf("detail view lacks the verified truth line:\n%s", out)
	}

	writeFile(t, e.bin+"/dxgi.dll", "FOREIGN-DXGI")
	e.sess.RefreshInstallState(e.gameRoot)
	pollUntil(t, "truth to report the drift", func() bool {
		if dt := e.sess.Snapshot().DetailTruth; dt != nil {
			return dt.Changed == 1
		}
		return false
	})
	out := m.detailView(100, 30)
	if !strings.Contains(out, "1 changed") || !strings.Contains(out, "differs from manager records") {
		t.Fatalf("detail view lacks the drift truth line:\n%s", out)
	}
	if got := findRow(e.sess.Snapshot().Rows, e.gameRoot).Status; got != "committed" {
		t.Fatalf("row status = %q, want committed (stale record beside live truth)", got)
	}
	t.Log("tui detail: verified line, then drift line beside committed status")
}
