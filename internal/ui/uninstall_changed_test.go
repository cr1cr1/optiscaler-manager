package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// TestUninstallChangedFilesConfirmFlow: when a tracked file no longer
// matches the manifest (foreign modification — another fork's bytes
// extracted over the managed install), a standalone uninstall must not
// hard-fail — it pauses with a confirmation naming the changed files;
// declining leaves every byte in place, accepting drops the foreign
// bytes and restores the adopt-time original (issue 076).
func TestUninstallChangedFilesConfirmFlow(t *testing.T) {
	e := newTestEnv(t)
	writeUIFile(t, filepath.Join(e.bin, "dxgi.dll"), "EXTERNAL-DXGI")
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)

	// The issue-076 scenario: foreign bytes land over the managed file.
	writeUIFile(t, filepath.Join(e.bin, "dxgi.dll"), "FOREIGN-DXGI")

	e.sess.Uninstall(e.gameRoot)
	waitEvent(t, e.sess, EvConfirm)
	st := e.sess.Snapshot()
	if st.Confirm == nil || st.Confirm.Kind != ConfirmChangedFiles {
		t.Fatalf("expected pending ConfirmChangedFiles, got %+v", st.Confirm)
	}
	if !strings.Contains(st.Confirm.Message, "dxgi.dll") {
		t.Errorf("consent message %q does not name the changed file", st.Confirm.Message)
	}
	data, err := os.ReadFile(filepath.Join(e.bin, "dxgi.dll"))
	if err != nil || string(data) != "FOREIGN-DXGI" {
		t.Fatalf("uninstall touched the changed file before consent: %q, %v", data, err)
	}

	// Decline: confirm cleared, foreign bytes untouched.
	e.sess.AnswerConfirm(false)
	if st := e.sess.Snapshot(); st.Confirm != nil {
		t.Fatal("declined confirmation was not cleared")
	}
	data, err = os.ReadFile(filepath.Join(e.bin, "dxgi.dll"))
	if err != nil || string(data) != "FOREIGN-DXGI" {
		t.Fatalf("declined uninstall touched the changed file: %q, %v", data, err)
	}

	// Re-ask and accept: foreign bytes dropped, adopt-time original
	// restored from the backup store, uninstall completes.
	e.sess.Uninstall(e.gameRoot)
	waitEvent(t, e.sess, EvConfirm)
	e.sess.AnswerConfirm(true)
	waitEvent(t, e.sess, EvOpDone)
	data, err = os.ReadFile(filepath.Join(e.bin, "dxgi.dll"))
	if err != nil || string(data) != "EXTERNAL-DXGI" {
		t.Fatalf("consented uninstall = %q, %v; want EXTERNAL-DXGI restored", data, err)
	}
	if got := e.sess.Snapshot().Rows[0].Status; got == domain.StatusCommitted {
		t.Fatalf("row still committed after consented uninstall")
	}
	t.Log("changed-files gate: blocked, declined, consented, original restored")
}
