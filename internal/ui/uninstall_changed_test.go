package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestUninstallChangedAndMissingBackupGatesChain: when BOTH consent
// conditions hold (foreign-modified file AND its backup lost — the real
// Witcher 3 state behind issue 076), the two gates must chain, not
// ping-pong: accepting the first resume re-pauses on the second, and
// accepting that completes the uninstall. The issue-076 resume dropped
// the other consent, looping the gates forever (issue 077).
func TestUninstallChangedAndMissingBackupGatesChain(t *testing.T) {
	e := newTestEnv(t)
	writeUIFile(t, filepath.Join(e.bin, "dxgi.dll"), "EXTERNAL-DXGI")
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)

	// Foreign bytes over the managed file AND the backup store lost.
	writeUIFile(t, filepath.Join(e.bin, "dxgi.dll"), "FOREIGN-DXGI")
	if err := os.Remove(filepath.Join(e.bin, "optiscaler-backups", "files", "dxgi.dll")); err != nil {
		t.Fatal(err)
	}

	// The refusal check precedes the backup probe inside the overwritten
	// loop, so the changed-files gate fires first; the consented retry
	// then surfaces the missing backup.
	e.sess.Uninstall(e.gameRoot)
	waitEvent(t, e.sess, EvConfirm)
	if got := e.sess.Snapshot().Confirm.Kind; got != ConfirmChangedFiles {
		t.Fatalf("first gate = %v, want ConfirmChangedFiles", got)
	}
	e.sess.AnswerConfirm(true)
	waitEvent(t, e.sess, EvConfirm)
	if got := e.sess.Snapshot().Confirm.Kind; got != ConfirmMissingBackup {
		t.Fatalf("second gate = %v, want ConfirmMissingBackup", got)
	}
	e.sess.AnswerConfirm(true)

	// Both consents granted: the operation completes — a third gate
	// means the resume dropped a consent (the issue-077 loop).
	deadline := time.After(15 * time.Second)
loop:
	for {
		select {
		case ev := <-e.sess.Events():
			t.Logf("event: %v %q", ev.Kind, ev.Text)
			if ev.Kind == EvConfirm {
				t.Fatalf("a third gate fired after both consents: %q (a consent was lost across gates)", ev.Text)
			}
			if ev.Kind == EvOpDone {
				break loop
			}
		case <-deadline:
			t.Fatal("no terminal event after both consents")
		}
	}
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); !os.IsNotExist(err) {
		t.Fatal("changed file with lost backup survived double-consented uninstall")
	}
	if got := e.sess.Snapshot().Rows[0].Status; got == domain.StatusCommitted {
		t.Fatal("row still committed after double-consented uninstall")
	}
	t.Log("gate chain: missing-backup, changed-files, done — no ping-pong")
}
