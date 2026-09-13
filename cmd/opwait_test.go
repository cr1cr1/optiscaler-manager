package optiscalermanager

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// pollForRows waits until the session's snapshot carries n rows.
func pollForRows(t *testing.T, sess *ui.Session, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for len(sess.Snapshot().Rows) != n {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d rows (have %d)", n, len(sess.Snapshot().Rows))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// cmdQuickInstall is the command tests' fixture installer: boots a one-shot
// session, QuickInstalls dir, and waits for the done event.
func cmdQuickInstall(t *testing.T, d *Deps, dir string) {
	t.Helper()
	sess := newSession(d)
	sess.Start(cmdContext())
	pollForRows(t, sess, 1)
	sess.QuickInstall(dir)
	ev, err := waitForOp(sess, dir, 60*time.Second, io.Discard)
	if err != nil {
		t.Fatalf("fixture QuickInstall: %v", err)
	}
	if ev.Kind != ui.EvOpDone {
		t.Fatalf("fixture QuickInstall event = %v %q, want done", ev.Kind, ev.Text)
	}
}

// TestWaitForOpReturnsDone drives a real QuickInstall through a session
// built the way the commands build it: the waiter returns the done event
// and the install has landed on disk.
func TestWaitForOpReturnsDone(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, _ := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	sess := newSession(d)
	sess.Start(cmdContext())
	pollForRows(t, sess, 1)

	sess.QuickInstall(gameRoot)
	ev, err := waitForOp(sess, gameRoot, 30*time.Second, io.Discard)
	if err != nil {
		t.Fatalf("waitForOp: %v", err)
	}
	if ev.Kind != ui.EvOpDone {
		t.Fatalf("event kind = %v, want EvOpDone (%q)", ev.Kind, ev.Text)
	}
	if _, serr := os.Stat(filepath.Join(gameRoot, "bin", "dxgi.dll")); serr != nil {
		t.Fatalf("install did not land files: %v", serr)
	}
	t.Logf("done event: %q", ev.Text)
}

// TestWaitForOpNonInteractiveDeclinesConsent: an EAC-protected game pauses
// on the consent gate before the op registers; stdin is /dev/null under go
// test, so the gate must be declined — the waiter ends with a "declined"
// error and the EvConfirm event, and no file may land (consent is never
// bypassed on the CLI).
func TestWaitForOpNonInteractiveDeclinesConsent(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	writeCmdTestFile(t, filepath.Join(gameRoot, "start_protected_game.exe"), "EAC")
	d, _ := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	sess := newSession(d)
	sess.Start(cmdContext())
	pollForRows(t, sess, 1)

	sess.QuickInstall(gameRoot)
	ev, err := waitForOp(sess, gameRoot, 30*time.Second, io.Discard)
	if err == nil {
		t.Fatal("waitForOp must fail when consent is declined")
	}
	if !strings.Contains(err.Error(), "declined") {
		t.Errorf("err = %v, want a declined explanation", err)
	}
	if ev.Kind != ui.EvConfirm {
		t.Errorf("event kind = %v, want EvConfirm (the declined gate)", ev.Kind)
	}
	if _, serr := os.Stat(filepath.Join(gameRoot, "bin", "dxgi.dll")); !os.IsNotExist(serr) {
		t.Fatal("install proceeded without consent")
	}
	t.Logf("declined non-interactive consent: err=%v event=%v %q", err, ev.Kind, ev.Text)
}

// TestWaitForOpAcceptedConsentInstalls pins the accept path end-to-end: the
// injected acceptor answers the EAC gate and the install completes, files
// landed, EvOpDone returned.
func TestWaitForOpAcceptedConsentInstalls(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	writeCmdTestFile(t, filepath.Join(gameRoot, "start_protected_game.exe"), "EAC")
	d, _ := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	sess := newSession(d)
	sess.Start(cmdContext())
	pollForRows(t, sess, 1)

	origGate := answerGateFn
	// answerGateFn's contract includes ANSWERING the gate (like the real
	// prompt): accept it.
	answerGateFn = func(s *ui.Session, _ io.Writer) bool {
		s.AnswerConfirm(true)
		return true
	}
	t.Cleanup(func() { answerGateFn = origGate })

	sess.QuickInstall(gameRoot)
	ev, err := waitForOp(sess, gameRoot, 30*time.Second, io.Discard)
	if err != nil {
		t.Fatalf("waitForOp: %v", err)
	}
	if ev.Kind != ui.EvOpDone {
		t.Fatalf("event kind = %v, want EvOpDone (%q)", ev.Kind, ev.Text)
	}
	if _, serr := os.Stat(filepath.Join(gameRoot, "bin", "dxgi.dll")); serr != nil {
		t.Fatalf("accepted install did not land files: %v", serr)
	}
	t.Logf("accepted consent installed: %q", ev.Text)
}
