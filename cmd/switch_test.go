package optiscalermanager

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// fakeGitHubTwoReleases serves two non-prerelease releases (list head
// v0.9.4-test = what "latest" resolves to, plus v0.10.0-test) so a switch
// between them can be exercised; same bundle fixture as fakeGitHub.
func fakeGitHubTwoReleases(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.9.4-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]},{"tag_name":"v0.10.0-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]}]`, srv.URL+"/bundle", srv.URL+"/bundle")
	})
	mux.HandleFunc("/bundle", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("..", "internal", "installer", "testdata", "bundle.7z"))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestSwitchCommandResolvesLatest: with the row committed at the fixture
// head, switching to the concrete v0.10.0-test installs it; switching to
// "latest" re-resolves at pick time to the list head (v0.9.4-test) — the
// v0.15 core seam, no new CLI logic.
func TestSwitchCommandResolvesLatest(t *testing.T) {
	srv := fakeGitHubTwoReleases(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot

	cmdQuickInstall(t, d, gameRoot)

	if err := (&SwitchCmd{Path: gameRoot, Version: "v0.10.0-test"}).Run(d); err != nil {
		t.Fatalf("concrete switch: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "switched") || !strings.Contains(got, "v0.10.0-test") {
		t.Errorf("concrete switch output:\n%s", got)
	}

	if err := (&SwitchCmd{Path: gameRoot, Version: "latest"}).Run(d); err != nil {
		t.Fatalf("latest switch: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "switched") || !strings.Contains(got, "v0.9.4-test") {
		t.Errorf("latest switch output:\n%s", got)
	}
	t.Logf("full switch transcript:\n%s", out.String())
}

// TestSwitchCommandSameVersionNoOp (S13 parity): switching to the version
// already installed reports "already at" and succeeds without claiming a
// switch. The command pre-checks the literal before dispatching (the
// core's same-version switch is a silent no-op).
func TestSwitchCommandSameVersionNoOp(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot

	cmdQuickInstall(t, d, gameRoot)
	out.Reset()

	if err := (&SwitchCmd{Path: gameRoot, Version: "v0.9.4-test"}).Run(d); err != nil {
		t.Fatalf("same-version switch must succeed: %v", err)
	}
	if got := out.String(); strings.Contains(got, "switched") {
		t.Errorf("same-version switch must not claim a switch:\n%s", got)
	}
	if !strings.Contains(out.String(), "already at") {
		t.Errorf("same-version switch output:\n%s", out.String())
	}
}

// TestSwitchCommandSameVersionLatestNoOp pins the core's terminal event on
// the RESOLVED same-version no-op ("latest" resolves to the installed tag
// after the command's literal pre-check cannot see it): the one-shot CLI
// must settle quickly instead of waiting out its timeout.
func TestSwitchCommandSameVersionLatestNoOp(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot

	cmdQuickInstall(t, d, gameRoot)
	out.Reset()

	if err := (&SwitchCmd{Path: gameRoot, Version: "latest", Timeout: 5 * time.Second}).Run(d); err != nil {
		t.Fatalf("latest no-op switch must succeed: %v", err)
	}
	if got := out.String(); strings.Contains(got, "switched") || !strings.Contains(got, "already at") {
		t.Errorf("latest no-op switch output:\n%s", got)
	}
}

// TestSwitchCommandUnknownDirErrors: an unknown game dir is a runtime
// failure with exit code 1, not a silent no-op.
func TestSwitchCommandUnknownDirErrors(t *testing.T) {
	srv := fakeGitHub(t)
	steamRoot, _ := fakeSteam(t)
	d, _ := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot

	err := (&SwitchCmd{Path: filepath.Join(steamRoot, "nowhere"), Version: "latest"}).Run(d)
	exit, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("err = %v, want *ExitError", err)
	}
	if exit.Code != 1 {
		t.Errorf("exit code = %d, want 1", exit.Code)
	}
}

// fakeGitHubEmpty serves an empty releases list: "latest" cannot resolve.
func fakeGitHubEmpty(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestSwitchCommandUnresolvableLatestFails pins the failure half of the
// settle contract: with no releases to resolve, the switch chain settles
// once with the reason and the command exits 1 — it must not hang.
func TestSwitchCommandUnresolvableLatestFails(t *testing.T) {
	srv := fakeGitHubEmpty(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, _ := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot

	err := (&SwitchCmd{Path: gameRoot, Version: "latest", Timeout: 30 * time.Second}).Run(d)
	exit, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("err = %v, want *ExitError", err)
	}
	if exit.Code != 1 {
		t.Errorf("exit code = %d, want 1", exit.Code)
	}
	if !strings.Contains(exit.Error(), "cannot resolve") {
		t.Errorf("err = %v, want the cannot-resolve reason", exit.Error())
	}
	t.Logf("unresolvable latest: %v", err)
}

// TestSwitchCommandEACGateAcceptResumesAndSettles pins the load-bearing
// resume path: an accepted ConfirmVersionSwitch resumes the chain WRAPPER
// (doSwitchVersion, not the chain), so the accepted op still settles with
// exactly one EvOpSettled at the end — if a regression resumes the chain
// directly, the CLI would wait out its timeout with no settle.
func TestSwitchCommandEACGateAcceptResumesAndSettles(t *testing.T) {
	srv := fakeGitHubTwoReleases(t)
	steamRoot, gameRoot := fakeSteam(t)
	writeCmdTestFile(t, filepath.Join(gameRoot, "start_protected_game.exe"), "EAC")
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot

	origGate := answerGateFn
	// The real prompt accepts y from a TTY; the seam answers the gate
	// itself (its contract) and reports acceptance.
	answerGateFn = func(s *ui.Session, _ io.Writer) bool {
		s.AnswerConfirm(true)
		return true
	}
	t.Cleanup(func() { answerGateFn = origGate })

	// The install leg of cmdQuickInstall pauses on the same EAC gate and
	// is resumed by the acceptor — the fixture game ends up installed.
	cmdQuickInstall(t, d, gameRoot)

	if err := (&SwitchCmd{Path: gameRoot, Version: "v0.10.0-test", Timeout: 30 * time.Second}).Run(d); err != nil {
		t.Fatalf("switch through the accepted EAC gate: %v", err)
	}
	if !strings.Contains(out.String(), "switched") {
		t.Errorf("output =\n%s, want the switched report", out.String())
	}
	t.Logf("EAC-accepted switch output:\n%s", out.String())
}

// TestSwitchCommandEmptyVersionUsesDefault: an empty --version means the
// configured default version (the settings value), not a dispatch error.
func TestSwitchCommandEmptyVersionUsesDefault(t *testing.T) {
	srv := fakeGitHubTwoReleases(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	prefs := settings.Defaults()
	prefs.OnlineLookups = false
	prefs.DefaultVersion = "v0.9.4-test"
	if err := settings.Save(d.DataRoot, prefs); err != nil {
		t.Fatalf("save fixture settings: %v", err)
	}
	cmdQuickInstall(t, d, gameRoot)

	if err := (&SwitchCmd{Path: gameRoot, Timeout: 30 * time.Second}).Run(d); err != nil {
		t.Fatalf("empty-version switch: %v", err)
	}
	sess := newSession(d)
	sess.Start(cmdContext())
	pollForRows(t, sess, 1)
	row, ok := rowOfSession(sess, gameRoot)
	if !ok {
		t.Fatalf("row for %s missing", gameRoot)
	}
	if row.OptiScalerVersion != "v0.9.4-test" {
		t.Errorf("installed version = %s, want the configured default v0.9.4-test", row.OptiScalerVersion)
	}
	t.Logf("default-version switch output:\n%s", out.String())
}

// seedCmdGamesCache writes a warm games cache containing only a phantom
// row (same schema convention as the TUI tests; version must track ui's
// cacheSchemaVersion).
func seedCmdGamesCache(t *testing.T, root, phantomDir string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(struct {
		Version int          `json:"version"`
		Rows    []ui.GameRow `json:"rows"`
	}{Version: 6, Rows: []ui.GameRow{{Title: "Phantom", InstallDir: phantomDir}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "games.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSwitchCommandRescansWarmCacheMissingGame pins awaitRow's rescan: a
// warm cache that predates the game must not dead-end in "not found" —
// the command rescans once and finds the fixture game.
func TestSwitchCommandRescansWarmCacheMissingGame(t *testing.T) {
	srv := fakeGitHubTwoReleases(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	cmdQuickInstall(t, d, gameRoot)
	seedCmdGamesCache(t, d.DataRoot, filepath.Join(steamRoot, "phantom"))

	if err := (&SwitchCmd{Path: gameRoot, Version: "v0.9.4-test", Timeout: 30 * time.Second}).Run(d); err != nil {
		t.Fatalf("switch after warm-cache rescan: %v", err)
	}
	if !strings.Contains(out.String(), "already at") {
		t.Errorf("output =\n%s, want the already-at report", out.String())
	}
	t.Logf("rescan switch output:\n%s", out.String())
}

// TestSwitchCommandFollowsSymlinkedPath: the CLI path is matched against
// the rows' canonical install dirs — a symlinked library path must work.
func TestSwitchCommandFollowsSymlinkedPath(t *testing.T) {
	srv := fakeGitHubTwoReleases(t)
	steamRoot, gameRoot := fakeSteam(t)
	d, out := testDeps(t, gh.NewWithBaseURL(nil, filepath.Join(t.TempDir(), "cache"), srv.URL))
	d.SteamRoot = steamRoot
	cmdQuickInstall(t, d, gameRoot)

	link := filepath.Join(t.TempDir(), "linked-game")
	if err := os.Symlink(gameRoot, link); err != nil {
		t.Fatalf("symlink fixture: %v", err)
	}

	err := (&SwitchCmd{Path: link, Version: "v0.10.0-test", Timeout: 30 * time.Second}).Run(d)
	if err != nil {
		t.Fatalf("switch via symlinked path: %v", err)
	}
	if !strings.Contains(out.String(), "switched") {
		t.Errorf("output =\n%s, want the switched report", out.String())
	}
	t.Logf("symlinked-path switch output:\n%s", out.String())
}
