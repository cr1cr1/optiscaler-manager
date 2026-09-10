package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// dlssEnv extends the standard session fake with a DLSS client pointed at a
// fake NVIDIA endpoint and the three NVIDIA DLLs planted in the game's
// injection directory.
type dlssEnv struct {
	*testEnv
	dlssRoot string
}

func newDLSEnv(t *testing.T, withDLLs bool) *dlssEnv {
	t.Helper()
	e := &dlssEnv{testEnv: newTestEnv(t), dlssRoot: t.TempDir()}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/NVIDIA/DLSS/commits/main", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sha":"` + strings.Repeat("d", 40) + `"}`))
	})
	mux.HandleFunc("/NVIDIA/DLSS/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(testutil.FixedVersionPE(310, 5, 3, 0))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	e.sess.deps.DLSS = dlss.NewWithBaseURLs(srv.Client(), srv.URL, srv.URL)
	e.sess.deps.SettingsRoot = e.dlssRoot
	if withDLLs {
		for i, name := range dlss.Files {
			writeUIFile(t, filepath.Join(e.bin, name), string(testutil.FixedVersionPE(3, 7, uint16(20+i), 0)))
		}
	}
	return e
}

func scanOneDLSSRow(t *testing.T, e *dlssEnv) GameRow {
	t.Helper()
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	rows := e.sess.Snapshot().Rows
	if len(rows) != 1 {
		t.Fatalf("rows %d, want 1", len(rows))
	}
	return rows[0]
}

func dlssDLLVersion(t *testing.T, e *dlssEnv, name string) string {
	t.Helper()
	v, err := pever.FileVersion(filepath.Join(e.bin, name))
	if err != nil {
		t.Fatalf("%s unreadable after op: %v", name, err)
	}
	return v
}

// TestUpdateDLSSAndRestoreRoundTrip: the update replaces all three NVIDIA
// DLLs from one immutable commit after backing them up, the row's DLSS pill
// version refreshes, and a confirmed restore brings the complete prior set
// back.
func TestUpdateDLSSAndRestoreRoundTrip(t *testing.T) {
	e := newDLSEnv(t, true)
	row := scanOneDLSSRow(t, e)

	e.sess.UpdateDLSS(row.InstallDir)
	waitEvent(t, e.sess, EvOpDone)
	for _, name := range dlss.Files {
		if got := dlssDLLVersion(t, e, name); got != "310.5.3.0" {
			t.Errorf("%s after update = %q, want 310.5.3.0", name, got)
		}
	}
	updated := e.sess.Snapshot().Rows[0]
	if len(updated.Components) == 0 || updated.Components[0] != "DLSS 4.5" {
		t.Errorf("components after update %v, want [DLSS 4.5]", updated.Components)
	}

	snaps := e.sess.DLSSSnapshots(row.InstallDir)
	if len(snaps) != 1 {
		t.Fatalf("snapshots after update %d, want 1 (the pre-update set)", len(snaps))
	}
	e.sess.RestoreDLSS(row.InstallDir, snaps[0].ID)
	waitEvent(t, e.sess, EvConfirm)
	confirm := e.sess.Snapshot().Confirm
	if confirm == nil {
		t.Fatal("restore produced no confirmation")
	}
	if confirm.Kind != ConfirmDLSSRestore || confirm.SnapshotID != snaps[0].ID {
		t.Errorf("confirm %+v, want ConfirmDLSSRestore for %q", confirm, snaps[0].ID)
	}
	e.sess.AnswerConfirm(true)
	waitEvent(t, e.sess, EvOpDone)
	for i, name := range dlss.Files {
		want := fmt.Sprintf("3.7.%d.0", 20+i)
		if got := dlssDLLVersion(t, e, name); got != want {
			t.Errorf("%s after restore = %q, want %q (the backed-up original)", name, got, want)
		}
	}
	restored := e.sess.Snapshot().Rows[0]
	if len(restored.Components) == 0 || restored.Components[0] != "DLSS 3.7.20" {
		t.Errorf("components after restore %v, want [DLSS 3.7.20]", restored.Components)
	}
	// The restore itself backed up the updated set: a second menu entry.
	if got := len(e.sess.DLSSSnapshots(row.InstallDir)); got != 2 {
		t.Errorf("snapshots after restore %d, want 2", got)
	}
	t.Log("update backed up originals, restore swapped the complete set back")
}

// TestUpdateDLSSMissingDLLRefused: an incomplete NVIDIA set is never
// updated and never written to.
func TestUpdateDLSSMissingDLLRefused(t *testing.T) {
	e := newDLSEnv(t, false)
	row := scanOneDLSSRow(t, e)
	if err := os.WriteFile(filepath.Join(e.bin, dlss.Files[0]), testutil.FixedVersionPE(3, 7, 20, 0), 0o644); err != nil {
		t.Fatal(err)
	}

	e.sess.UpdateDLSS(row.InstallDir)
	ev := waitEvent(t, e.sess, EvOpFailed)
	if !strings.Contains(ev.Text, "missing") {
		t.Errorf("failure text %q, want a missing-DLL refusal", ev.Text)
	}
	if got := dlssDLLVersion(t, e, dlss.Files[0]); got != "3.7.20.0" {
		t.Errorf("%s mutated by refused update: %q", dlss.Files[0], got)
	}
	if snaps := e.sess.DLSSSnapshots(row.InstallDir); len(snaps) != 0 {
		t.Errorf("refused update created %d snapshots, want 0", len(snaps))
	}
}

// TestRestoreDLSSDeclineLeavesUntouched: declining the confirmation must
// run no operation and change no bytes.
func TestRestoreDLSSDeclineLeavesUntouched(t *testing.T) {
	e := newDLSEnv(t, true)
	row := scanOneDLSSRow(t, e)
	e.sess.RestoreDLSS(row.InstallDir, "nonexistent")
	if c := e.sess.Snapshot().Confirm; c != nil {
		t.Fatalf("unknown snapshot id opened a confirm: %+v", c)
	}
	// A real snapshot seeds the confirm; declining must be a no-op.
	e.sess.UpdateDLSS(row.InstallDir)
	waitEvent(t, e.sess, EvOpDone)
	snaps := e.sess.DLSSSnapshots(row.InstallDir)
	e.sess.RestoreDLSS(row.InstallDir, snaps[0].ID)
	if e.sess.Snapshot().Confirm == nil {
		t.Fatal("restore of a real snapshot produced no confirmation")
	}
	e.sess.AnswerConfirm(false)
	if c := e.sess.Snapshot().Confirm; c != nil {
		t.Fatalf("confirm not cleared on decline: %+v", c)
	}
	if got := dlssDLLVersion(t, e, dlss.Files[0]); got != "310.5.3.0" {
		t.Errorf("%s changed on declined restore: %q", dlss.Files[0], got)
	}
	if got := len(e.sess.DLSSSnapshots(row.InstallDir)); got != 1 {
		t.Errorf("declined restore created %d snapshots, want 1", got)
	}
}
