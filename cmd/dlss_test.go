package optiscalermanager

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// dlssCmdEnv is the DLSS command fixture: fake Steam + fake GitHub + fake
// NVIDIA, a committed row (via the real install path), and the three-file
// runtime set at 1.0.<i>.0. Returns the deps, the output buffer, and the
// injection dir.
func dlssCmdEnv(t *testing.T) (*Deps, *bytes.Buffer, string, [3][]byte) {
	t.Helper()
	srv := fakeGitHub(t)
	nv := fakeNVIDIA(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	d.DLSS = dlss.NewWithBaseURLs(nil, nv.URL, nv.URL)

	cmdQuickInstall(t, d, gameRoot)
	var old [3][]byte
	for i, name := range dlss.Files {
		old[i] = testutil.FixedVersionPE(1, 0, uint16(i), 0)
		writeCmdTestFile(t, filepath.Join(gameRoot, "bin", name), string(old[i]))
	}
	return d, out, gameRoot, old
}

// fakeNVIDIA serves the fake NVIDIA/DLSS endpoints: the main-branch commit
// resolves to a fixed sha and every raw file route serves a PE at 310.9.1.0.
func fakeNVIDIA(t *testing.T) *httptest.Server {
	t.Helper()
	sha := strings.Repeat("a", 40)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/NVIDIA/DLSS/commits/main", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sha":"` + sha + `"}`))
	})
	mux.HandleFunc("/NVIDIA/DLSS/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(testutil.FixedVersionPE(310, 9, 1, 0))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// wantRuntimeSet asserts all three DLLs carry exactly the wanted bytes.
func wantRuntimeSet(t *testing.T, gameRoot string, want [3][]byte) {
	t.Helper()
	for i, name := range dlss.Files {
		got, err := os.ReadFile(filepath.Join(gameRoot, "bin", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !bytes.Equal(got, want[i]) {
			t.Errorf("%s bytes changed, want the seeded version set", name)
		}
	}
}

// TestDLSSUpdateCommandUpdatesRuntime: dlss-update on a committed game
// replaces all three runtime DLLs with the published set and reports the
// outcome.
func TestDLSSUpdateCommandUpdatesRuntime(t *testing.T) {
	d, out, gameRoot, old := dlssCmdEnv(t)
	wantRuntimeSet(t, gameRoot, old)

	if err := (&DLSSUpdateCmd{Path: gameRoot}).Run(d); err != nil {
		t.Fatalf("DLSSUpdateCmd: %v", err)
	}
	var want [3][]byte
	for i := range want {
		want[i] = testutil.FixedVersionPE(310, 9, 1, 0)
	}
	wantRuntimeSet(t, gameRoot, want)
	if !strings.Contains(out.String(), "Updated") {
		t.Errorf("output missing the update report:\n%s", out.String())
	}
	t.Logf("update transcript:\n%s", out.String())
}

// TestDLSSRestoreCommandRestoresSnapshot: after an update, dlss-restore
// with the snapshot id (and with the empty default meaning the newest)
// puts the backed-up set back, behind the restore consent gate.
func TestDLSSRestoreCommandRestoresSnapshot(t *testing.T) {
	d, out, gameRoot, old := dlssCmdEnv(t)
	origGate := answerGateFn
	answerGateFn = func(s *ui.Session, _ io.Writer) bool {
		s.AnswerConfirm(true)
		return true
	}
	t.Cleanup(func() { answerGateFn = origGate })

	if err := (&DLSSUpdateCmd{Path: gameRoot}).Run(d); err != nil {
		t.Fatalf("update leg: %v", err)
	}
	snaps, serr := app.DLSSSnapshots(gameRoot)
	if serr != nil {
		t.Fatalf("list DLSS snapshots: %v", serr)
	}
	if len(snaps) == 0 {
		t.Fatal("no DLSS snapshot after the update")
	}

	if err := (&DLSSRestoreCmd{Path: gameRoot, Snapshot: snaps[0].ID}).Run(d); err != nil {
		t.Fatalf("explicit-id restore: %v", err)
	}
	wantRuntimeSet(t, gameRoot, old)

	// The empty id means the NEWEST snapshot. The explicit restore above
	// backed the current (310.9.1) set up first — that copy is now the
	// newest and differs from the on-disk old set, so restoring the
	// default must bring the 310.9.1 set back.
	if err := (&DLSSRestoreCmd{Path: gameRoot}).Run(d); err != nil {
		t.Fatalf("default-id restore: %v", err)
	}
	var newest [3][]byte
	for i := range newest {
		newest[i] = testutil.FixedVersionPE(310, 9, 1, 0)
	}
	wantRuntimeSet(t, gameRoot, newest)
	t.Logf("restore transcript:\n%s", out.String())
}

// TestDLSSRestoreCommandDeclinedNonInteractive: under go test stdin is not
// a terminal, so the restore consent gate must be declined — the runtime
// set stays untouched and the command fails.
func TestDLSSRestoreCommandDeclinedNonInteractive(t *testing.T) {
	d, _, gameRoot, _ := dlssCmdEnv(t)
	if err := (&DLSSUpdateCmd{Path: gameRoot}).Run(d); err != nil {
		t.Fatalf("update leg: %v", err)
	}
	snaps, serr := app.DLSSSnapshots(gameRoot)
	if serr != nil {
		t.Fatalf("list DLSS snapshots: %v", serr)
	}
	if len(snaps) == 0 {
		t.Fatal("no DLSS snapshot after the update")
	}

	err := (&DLSSRestoreCmd{Path: gameRoot, Snapshot: snaps[0].ID, Timeout: 30 * time.Second}).Run(d)
	if err == nil {
		t.Fatal("declined restore must fail")
	}
	if !strings.Contains(err.Error(), "declined") {
		t.Errorf("err = %v, want a declined explanation", err)
	}
	var updated [3][]byte
	for i := range updated {
		updated[i] = testutil.FixedVersionPE(310, 9, 1, 0)
	}
	wantRuntimeSet(t, gameRoot, updated)
}

// TestDLSSRestoreCommandUnknownSnapshotFails: an explicit id that no
// backup carries must fail FAST (exit 1) — the session only toasts on an
// unknown id, so dispatching it would hang the one-shot CLI.
func TestDLSSRestoreCommandUnknownSnapshotFails(t *testing.T) {
	d, _, gameRoot, _ := dlssCmdEnv(t)

	err := (&DLSSRestoreCmd{Path: gameRoot, Snapshot: "bogus-id", Timeout: 5 * time.Second}).Run(d)
	exit, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("err = %v, want *ExitError", err)
	}
	if exit.Code != 1 {
		t.Errorf("exit code = %d, want 1", exit.Code)
	}
	if !strings.Contains(exit.Error(), "unknown DLSS snapshot") {
		t.Errorf("err = %v, want the unknown-snapshot reason", exit.Error())
	}
	t.Logf("unknown snapshot id: %v", err)
}

// TestDLSSUpdateCommandFailureFails pins the op-failure half of the
// contract: with the runtime set in place but an unreachable NVIDIA
// endpoint, the update settles with EvOpFailed and the command exits 1
// with the reason instead of hanging.
func TestDLSSUpdateCommandFailureFails(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	d.DLSS = dlss.NewWithBaseURLs(nil, "http://127.0.0.1:1", "http://127.0.0.1:1")
	cmdQuickInstall(t, d, gameRoot)
	for _, name := range dlss.Files {
		writeCmdTestFile(t, filepath.Join(gameRoot, "bin", name), "runtime")
	}

	err := (&DLSSUpdateCmd{Path: gameRoot, Timeout: 30 * time.Second}).Run(d)
	exit, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("err = %v, want *ExitError", err)
	}
	if exit.Code != 1 {
		t.Errorf("exit code = %d, want 1", exit.Code)
	}
	t.Logf("failed update: %v; output:\n%s", err, out.String())
}
