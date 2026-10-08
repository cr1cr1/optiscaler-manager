package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// TestUninstallMissingBackupConfirmFlow: when an adopt-time backup entry
// is gone, a standalone uninstall must not hard-fail — it pauses with a
// confirmation naming the unrestorable files; declining leaves every byte
// in place, accepting removes the mod without restoring the lost
// original (issue 050).
func TestUninstallMissingBackupConfirmFlow(t *testing.T) {
	e := newTestEnv(t)
	writeUIFile(t, filepath.Join(e.bin, "dxgi.dll"), "EXTERNAL-DXGI")
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)

	// The issue-050 scenario: the backup store lost an entry (semi-manual
	// install switched across forks).
	if err := os.Remove(filepath.Join(e.bin, "optiscaler-backups", "files", "dxgi.dll")); err != nil {
		t.Fatal(err)
	}

	e.sess.Uninstall(e.gameRoot)
	waitEvent(t, e.sess, EvConfirm)
	st := e.sess.Snapshot()
	if st.Confirm == nil || st.Confirm.Kind != ConfirmMissingBackup {
		t.Fatalf("expected pending ConfirmMissingBackup, got %+v", st.Confirm)
	}
	if !strings.Contains(st.Confirm.Message, "dxgi.dll") {
		t.Errorf("consent message %q does not name the unrestorable file", st.Confirm.Message)
	}
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Fatal("uninstall removed files before consent")
	}

	// Decline: confirm cleared, files and manifest untouched.
	e.sess.AnswerConfirm(false)
	if st := e.sess.Snapshot(); st.Confirm != nil {
		t.Fatal("declined confirmation was not cleared")
	}
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Fatal("declined uninstall removed files")
	}

	// Re-ask and accept: mod bytes removed, uninstall completes.
	e.sess.Uninstall(e.gameRoot)
	waitEvent(t, e.sess, EvConfirm)
	e.sess.AnswerConfirm(true)
	waitEvent(t, e.sess, EvOpDone)
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); !os.IsNotExist(err) {
		t.Fatal("mod dxgi.dll survived consented removal")
	}
	if got := e.sess.Snapshot().Rows[0].Status; got == domain.StatusCommitted {
		t.Fatalf("row still committed after consented removal")
	}
	t.Log("missing-backup gate: blocked, declined, consented, removed")
}
