package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// TestSelectReconcilesDetailTruth: opening the details pane reconciles
// the game against its actual directory (issue 079). A foreign-modified
// file shows as changed in the published truth while the manifest still
// claims committed — the Witcher 3 scenario where the cached row status
// silently disagreed with reality.
func TestSelectReconcilesDetailTruth(t *testing.T) {
	e := newTestEnv(t)
	writeUIFile(t, filepath.Join(e.bin, "dxgi.dll"), "EXTERNAL-DXGI")
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)

	e.sess.Select(e.gameRoot)
	waitEvent(t, e.sess, EvDetailTruth)
	dt := e.sess.Snapshot().DetailTruth
	if dt == nil || dt.Dir != e.gameRoot {
		t.Fatalf("DetailTruth = %+v, want truth for %s", dt, e.gameRoot)
	}
	if !dt.Managed || dt.Tracked == 0 {
		t.Fatalf("truth = %+v, want a managed install with tracked files", dt)
	}
	if dt.Changed != 0 || dt.Verified != dt.Tracked {
		t.Fatalf("truth = %+v, want everything verified right after install", dt)
	}
	if got := TruthLine(dt); !strings.Contains(got, "verified") {
		t.Errorf("truth line %q does not read as verified", got)
	}

	// Foreign bytes land over a managed file; the manifest is untouched.
	writeUIFile(t, filepath.Join(e.bin, "dxgi.dll"), "FOREIGN-DXGI")
	e.sess.RefreshInstallState(e.gameRoot)
	waitEvent(t, e.sess, EvDetailTruth)
	dt = e.sess.Snapshot().DetailTruth
	if dt.Changed != 1 {
		t.Fatalf("Changed = %d, want 1 after the foreign edit", dt.Changed)
	}
	if got := e.sess.Snapshot().Rows[0].Status; got != domain.StatusCommitted {
		t.Fatalf("row status = %q, want committed (the manager record is stale, the truth is not)", got)
	}
	line := TruthLine(dt)
	if !strings.Contains(line, "1 changed") || !strings.Contains(line, "differs from manager records") {
		t.Errorf("truth line %q does not name the drift", line)
	}
	t.Log("details pane truth: verified, then 1 changed while committed")
}

// TestTruthLineShapes pins the shared details-pane wording both
// frontends render (issue 079).
func TestTruthLineShapes(t *testing.T) {
	if got := TruthLine(nil); got != "" {
		t.Errorf("nil truth line = %q, want empty", got)
	}
	clean := TruthLine(&DetailTruth{Dir: "g"})
	if !strings.Contains(clean, "no OptiScaler files detected") {
		t.Errorf("clean line = %q", clean)
	}
	t.Log("truth line shapes pinned")
}
