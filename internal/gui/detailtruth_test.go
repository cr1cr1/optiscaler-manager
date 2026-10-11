package gui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "go.hasen.dev/shirei"
)

// The detail panel renders the directory-verified install truth (issue
// 079): the manager's cached status is a claim, the "Game dir:" line is
// the game directory's word — verified files after install, and the named
// drift after a foreign edit while the manifest still claims committed.
func TestDetailPanelRendersGameDirTruth(t *testing.T) {
	sess, row := committedAtLatest(t)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	deadline := time.Now().Add(15 * time.Second)
	for sess.Snapshot().DetailTruth == nil && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	if sess.Snapshot().DetailTruth == nil {
		t.Fatal("no DetailTruth after Select (no EvDetailTruth reconciliation)")
	}

	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects from the previous frame
	if m.truthLineRect.Size[0] == 0 || m.truthLineRect.Size[1] == 0 {
		t.Fatalf("truth line not rendered for a managed row (rect %+v)", m.truthLineRect)
	}

	// The Witcher 3 scenario: foreign bytes over a tracked file (the
	// install's injection dir, not the game root). The row still claims
	// committed; the panel must render the drift truth.
	tracked := filepath.Join(row.InstallDir, "bin", "dxgi.dll")
	if err := os.WriteFile(tracked, []byte("FOREIGN-DXGI"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess.RefreshInstallState(row.InstallDir)
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if dt := sess.Snapshot().DetailTruth; dt != nil && dt.Changed == 1 {
			break
		}
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	if dt := sess.Snapshot().DetailTruth; dt == nil || dt.Changed != 1 {
		t.Fatalf("truth did not report the drift: %+v", dt)
	}
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.truthLineRect.Size[0] == 0 || m.truthLineRect.Size[1] == 0 {
		t.Fatalf("truth line vanished on drift (rect %+v)", m.truthLineRect)
	}
	t.Log("gui panel: truth line rendered verified, still rendered on drift")
}
