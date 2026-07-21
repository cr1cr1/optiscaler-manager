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
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// testEnv wires a Session against fakes: httptest GitHub + CDN, temp store,
// temp Steam root with one game.
type testEnv struct {
	sess      *Session
	steamRoot string
	gameRoot  string
	bin       string
	srv       *httptest.Server
	opened    []string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	e := &testEnv{}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.9.4-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]}]`, e.srv.URL+"/bundle")
	})
	mux.HandleFunc("/bundle", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("..", "installer", "testdata", "bundle.7z"))
	})
	mux.HandleFunc("/cdn/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // force placeholder covers in most tests
	})
	mux.HandleFunc("/search/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	e.srv = httptest.NewServer(mux)
	t.Cleanup(e.srv.Close)

	root := t.TempDir()
	e.steamRoot = t.TempDir()
	e.gameRoot = filepath.Join(e.steamRoot, "steamapps", "common", "GameOne")
	e.bin = filepath.Join(e.gameRoot, "bin")
	writeUIFile(t, filepath.Join(e.steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+e.steamRoot+`" } }`)
	writeUIFile(t, filepath.Join(e.steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`)
	writeUIFile(t, filepath.Join(e.bin, "gameone.exe"), "GAME")
	writeUIFile(t, filepath.Join(e.bin, "nvngx_dlss.dll"), "DLSS")

	ghClient := gh.NewWithBaseURL(nil, filepath.Join(root, "cache"), e.srv.URL)
	e.sess = NewSession(Deps{
		Store:     store.New(root),
		GH:        ghClient,
		Covers:    covers.NewWithBase(nil, filepath.Join(root, "covers"), e.srv.URL+"/cdn/%s", e.srv.URL+"/search/"),
		CacheDir:  filepath.Join(root, "cache"),
		SteamRoot: e.steamRoot,
	})
	e.sess.openExternal = func(path string) error {
		e.opened = append(e.opened, path)
		return nil
	}
	return e
}

func writeUIFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(strings.ToLower(path), ".exe") && !strings.HasPrefix(content, "MZ") {
		content = "MZ" + content
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// waitEvent drains the session's events until kind arrives or the deadline.
func waitEvent(t *testing.T, s *Session, kind EventKind) Event {
	t.Helper()
	deadline := time.After(90 * time.Second)
	for {
		select {
		case ev := <-s.Events():
			t.Logf("event: %v %q", ev.Kind, ev.Text)
			if ev.Kind == kind {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event %v", kind)
		}
	}
}

func TestScanPopulatesRows(t *testing.T) {
	e := newTestEnv(t)
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	st := e.sess.Snapshot()
	if len(st.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(st.Rows))
	}
	row := st.Rows[0]
	if row.Title != "Game One" || row.AppID != "100" {
		t.Errorf("bad row identity: %+v", row)
	}
	if len(row.TechBadges) != 1 || row.TechBadges[0].Label != "DLSS" || row.TechBadges[0].Tone != ToneGreen {
		t.Errorf("bad tech badges: %+v", row.TechBadges)
	}
	if row.CoverPath == "" {
		t.Error("cover path empty")
	}
	t.Logf("row: %+v", row)
}

func TestQuickInstallTogglesByStatus(t *testing.T) {
	e := newTestEnv(t)
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	// Not installed → QuickInstall installs.
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Fatalf("dxgi.dll not installed: %v", err)
	}
	if got := e.sess.Snapshot().Rows[0].Status; got != domain.StatusCommitted {
		t.Fatalf("row status %q after install, want committed", got)
	}

	// Installed → QuickInstall uninstalls.
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); !os.IsNotExist(err) {
		t.Fatal("dxgi.dll survived quick uninstall")
	}
	t.Log("quick install toggles both ways")
}

func TestEACConfirmBlocksInstall(t *testing.T) {
	e := newTestEnv(t)
	writeUIFile(t, filepath.Join(e.gameRoot, "start_protected_game.exe"), "EAC")
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvConfirm)

	st := e.sess.Snapshot()
	if st.Confirm == nil || st.Confirm.Kind != ConfirmEAC {
		t.Fatalf("expected pending EAC confirmation, got %+v", st.Confirm)
	}
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); !os.IsNotExist(err) {
		t.Fatal("install proceeded without confirmation")
	}

	e.sess.AnswerConfirm(false)
	if st := e.sess.Snapshot(); st.Confirm != nil {
		t.Fatal("declined confirmation was not cleared")
	}

	// Re-ask and accept this time.
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvConfirm)
	e.sess.AnswerConfirm(true)
	waitEvent(t, e.sess, EvOpDone)
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Fatal("install did not proceed after consent")
	}
	t.Log("EAC gate: blocked, declined, consented")
}

func TestStaleCacheRequiresConsent(t *testing.T) {
	e := newTestEnv(t)
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	// First install primes the release cache and starts the gh cooldown.
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)
	e.sess.QuickInstall(e.gameRoot) // uninstall again
	waitEvent(t, e.sess, EvOpDone)

	// Second install inside the cooldown must ask before using stale cache.
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvConfirm)
	if st := e.sess.Snapshot(); st.Confirm == nil || st.Confirm.Kind != ConfirmCachedRelease {
		t.Fatalf("expected cached-release confirmation, got %+v", st.Confirm)
	}
	e.sess.AnswerConfirm(true)
	waitEvent(t, e.sess, EvOpDone)
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Fatal("install did not proceed after cache consent")
	}
	t.Log("stale cache gate: asked, consented")
}

func TestToastLifecycle(t *testing.T) {
	e := newTestEnv(t)
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)

	st := e.sess.Snapshot()
	if len(st.Toasts) == 0 {
		t.Fatal("no toast after completed op")
	}
	t.Logf("toast: %q", st.Toasts[len(st.Toasts)-1].Text)

	// Advance the clock past expiry: toasts prune on snapshot.
	past := time.Now()
	e.sess.now = func() time.Time { return past.Add(toastTTL + time.Minute) }
	if st := e.sess.Snapshot(); len(st.Toasts) != 0 {
		t.Fatalf("expired toasts not pruned: %d", len(st.Toasts))
	}
}

func TestOpenINICallsOpener(t *testing.T) {
	e := newTestEnv(t)
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)

	e.sess.OpenINI(e.gameRoot)
	if len(e.opened) != 1 || !strings.HasSuffix(e.opened[0], "OptiScaler.ini") {
		t.Fatalf("opener calls: %v", e.opened)
	}
	t.Logf("opened %s", e.opened[0])
}

func TestVisibleRowsFilterAndSort(t *testing.T) {
	now := time.Now()
	rows := []GameRow{
		{Title: "Bravo", AppID: "2", ModTime: now},
		{Title: "Alpha", AppID: "1", Status: domain.StatusFailed, Actionable: true, ModTime: now.Add(-time.Hour)},
		{Title: "Charlie", AppID: "3", ModTime: now.Add(-time.Minute)},
	}
	sorted := sortRows(rows)
	if sorted[0].Title != "Alpha" || sorted[1].Title != "Bravo" || sorted[2].Title != "Charlie" {
		t.Errorf("sort order: %v %v %v", sorted[0].Title, sorted[1].Title, sorted[2].Title)
	}
	filtered := filterRows(rows, "char")
	if len(filtered) != 1 || filtered[0].Title != "Charlie" {
		t.Errorf("filter: %+v", filtered)
	}
	if got := filterRows(rows, "3"); len(got) != 1 || got[0].AppID != "3" {
		t.Errorf("appid filter: %+v", got)
	}
	t.Log("sort + filter semantics preserved from v0.1 view model")
}

func TestSetDefaultVersionPersistsAndApplies(t *testing.T) {
	e := newTestEnv(t)
	root := t.TempDir()
	e.sess.deps.SettingsRoot = root

	e.sess.SetDefaultVersion("v0.9.4-test")
	if got := e.sess.Settings().DefaultVersion; got != "v0.9.4-test" {
		t.Fatalf("DefaultVersion %q", got)
	}
	loaded, err := settings.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.DefaultVersion != "v0.9.4-test" {
		t.Errorf("persisted version %q", loaded.DefaultVersion)
	}

	// The next install resolves the configured tag, recorded in the manifest.
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvOpDone)
	manifests, err := e.sess.deps.Store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests: %d %v", len(manifests), err)
	}
	if manifests[0].RequestedVersion != "v0.9.4-test" {
		t.Errorf("manifest RequestedVersion %q", manifests[0].RequestedVersion)
	}
	t.Log("default version persisted and applied to install")
}

// TestOpBusyReflectsInFlightOp: the dashboard gates its per-game Cancel
// button on OpBusy, so it must track exactly the game with an in-flight op.
func TestOpBusyReflectsInFlightOp(t *testing.T) {
	e := newTestEnv(t)
	if e.sess.OpBusy(e.gameRoot) {
		t.Fatal("OpBusy true before any op")
	}
	if _, ok := e.sess.registerOp(e.gameRoot); !ok {
		t.Fatal("registerOp refused a free slot")
	}
	if !e.sess.OpBusy(e.gameRoot) {
		t.Fatal("OpBusy false with an op in flight")
	}
	if e.sess.OpBusy("/some/other/game") {
		t.Fatal("OpBusy true for a different game")
	}
	e.sess.finishOp(e.gameRoot)
	if e.sess.OpBusy(e.gameRoot) {
		t.Fatal("OpBusy true after the op settled")
	}
}

func TestClearBundleCache(t *testing.T) {
	e := newTestEnv(t)
	dir := filepath.Join(e.sess.deps.CacheDir, "optiscaler", "v1")
	writeUIFile(t, filepath.Join(dir, "bundle.7z"), "cached")

	e.sess.ClearBundleCache()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(e.sess.deps.CacheDir, "optiscaler")); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cache dir still exists after clear")
		}
		time.Sleep(10 * time.Millisecond)
	}
	deadline = time.Now().Add(5 * time.Second)
	for len(e.sess.Snapshot().Toasts) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no confirmation toast")
		}
		time.Sleep(10 * time.Millisecond)
	}
	st := e.sess.Snapshot()
	t.Logf("cache cleared, toast: %q", st.Toasts[len(st.Toasts)-1].Text)
}

func TestAddDirectoryAddsRowAndPersists(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	custom := filepath.Join(t.TempDir(), "MyGame")
	writeUIFile(t, filepath.Join(custom, "game.exe"), "GAME")

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	if got := len(e.sess.Snapshot().Rows); got != 1 {
		t.Fatalf("rows before add: %d", got)
	}

	e.sess.AddDirectory(custom)
	waitEventText(t, e.sess, EvScanDone, "directory added")
	st := e.sess.Snapshot()
	if len(st.Rows) != 2 {
		t.Fatalf("rows after add: %d", len(st.Rows))
	}
	var found bool
	for _, r := range st.Rows {
		if r.InstallDir == custom && r.Title == "MyGame" {
			found = true
		}
	}
	if !found {
		t.Fatalf("custom game row missing: %+v", st.Rows)
	}
	loaded, _ := settings.Load(e.sess.deps.SettingsRoot)
	if len(loaded.ExtraDirs) != 1 || loaded.ExtraDirs[0] != custom {
		t.Fatalf("persisted ExtraDirs: %v", loaded.ExtraDirs)
	}

	// Duplicate add is a no-op (still 2 rows, 1 persisted dir).
	e.sess.AddDirectory(custom)
	if got := len(e.sess.Snapshot().Rows); got != 2 {
		t.Fatalf("rows after duplicate add: %d", got)
	}
	loaded, _ = settings.Load(e.sess.deps.SettingsRoot)
	if len(loaded.ExtraDirs) != 1 {
		t.Fatalf("ExtraDirs after duplicate: %v", loaded.ExtraDirs)
	}

	// A later scan keeps the manually added game.
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	if got := len(e.sess.Snapshot().Rows); got != 2 {
		t.Fatalf("rows after rescan: %d", got)
	}
	t.Log("manual add: row added, persisted, deduped, survives rescan")
}

func TestPickAndAddDirectory(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	custom := filepath.Join(t.TempDir(), "PickedGame")
	writeUIFile(t, filepath.Join(custom, "game.exe"), "GAME")
	e.sess.pickDir = func(ctx context.Context) (string, error) { return custom, nil }

	e.sess.PickAndAddDirectory(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	found := false
	for _, r := range e.sess.Snapshot().Rows {
		if r.InstallDir == custom {
			found = true
		}
	}
	if !found {
		t.Fatal("picked directory was not added")
	}
	t.Log("picker result added to library")
}
