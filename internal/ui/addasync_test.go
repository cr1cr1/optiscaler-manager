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
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// newSlowCoversEnv wires a Session whose cover CDN sleeps before answering,
// so enrichment work takes measurably longer than any acceptable caller
// blocking budget.
func newSlowCoversEnv(t *testing.T, delay time.Duration) *testEnv {
	t.Helper()
	e := &testEnv{}
	mux := http.NewServeMux()
	mux.HandleFunc("/cdn/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/search/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
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

	e.sess = NewSession(Deps{
		Store:     store.New(root),
		Covers:    covers.NewWithBase(nil, filepath.Join(root, "covers"), e.srv.URL+"/cdn/%s", e.srv.URL+"/search/"),
		CacheDir:  filepath.Join(root, "cache"),
		SteamRoot: e.steamRoot,
	})
	return e
}

// waitEventText drains until an event of kind with text arrives.
func waitEventText(t *testing.T, s *Session, kind EventKind, text string) Event {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case ev := <-s.Events():
			t.Logf("event: %v %q", ev.Kind, ev.Text)
			if ev.Kind == kind && ev.Text == text {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event %v %q", kind, text)
		}
	}
}

// TestAddDirectory_AsyncReturnsImmediately: AddDirectory must return while
// the enrichment (walk + classify + cover fetch) is still in flight; the
// enriched row settles via the usual "directory added" event.
func TestAddDirectory_AsyncReturnsImmediately(t *testing.T) {
	e := newSlowCoversEnv(t, 2*time.Second)
	e.sess.deps.SettingsRoot = t.TempDir()
	custom := filepath.Join(t.TempDir(), "SlowGame")
	writeUIFile(t, filepath.Join(custom, "game.exe"), "GAME")

	start := time.Now()
	e.sess.AddDirectory(custom)
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("AddDirectory blocked %v (cover fetch alone takes 2s)", elapsed)
	}

	waitEventText(t, e.sess, EvScanDone, "directory added")
	var row *GameRow
	for i, r := range e.sess.Snapshot().Rows {
		if r.InstallDir == custom {
			row = &e.sess.st.Rows[i]
			break
		}
	}
	if row == nil {
		t.Fatal("added row missing after completion event")
	}
	if row.CoverPath == "" {
		t.Error("row not enriched: CoverPath empty after completion event")
	}
	t.Logf("returned in %v, enriched row settled via event", elapsed)
}

// TestAddDirectory_DuplicateInFlightRejected: a second Add of the same
// canonical dir while the first is still enriching is rejected early with a
// toast and emits no completion event.
func TestAddDirectory_DuplicateInFlightRejected(t *testing.T) {
	e := newSlowCoversEnv(t, time.Second)
	e.sess.deps.SettingsRoot = t.TempDir()
	custom := filepath.Join(t.TempDir(), "DupGame")
	writeUIFile(t, filepath.Join(custom, "game.exe"), "GAME")

	e.sess.AddDirectory(custom)
	start := time.Now()
	e.sess.AddDirectory(custom) // duplicate while in flight
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("duplicate AddDirectory blocked %v", elapsed)
	}

	found := false
	for _, toast := range e.sess.Snapshot().Toasts {
		if strings.Contains(toast.Text, "add already in progress") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing rejection toast: %+v", e.sess.Snapshot().Toasts)
	}

	waitEventText(t, e.sess, EvScanDone, "directory added")
	// No second completion event may arrive for the rejected duplicate.
	deadline := time.After(300 * time.Millisecond)
	for {
		select {
		case ev := <-e.sess.Events():
			if ev.Kind == EvScanDone && ev.Text == "directory added" {
				t.Fatal("rejected duplicate emitted a completion event")
			}
		case <-deadline:
			count := 0
			for _, r := range e.sess.Snapshot().Rows {
				if r.InstallDir == custom {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("rows for dir = %d, want 1", count)
			}
			return
		}
	}
}

// TestAddDirectory_ConcurrentScanSerialized: an Add landing while a Scan is
// in flight must not corrupt rows — both complete and the final row set
// holds each game exactly once.
func TestAddDirectory_ConcurrentScanSerialized(t *testing.T) {
	e := newSlowCoversEnv(t, 300*time.Millisecond)
	e.sess.deps.SettingsRoot = t.TempDir()
	custom := filepath.Join(t.TempDir(), "DuringScan")
	writeUIFile(t, filepath.Join(custom, "game.exe"), "GAME")

	e.sess.Scan(context.Background())
	e.sess.AddDirectory(custom)

	waitEventText(t, e.sess, EvScanDone, "directory added")
	waitEvent(t, e.sess, EvScanDone) // scan completion (any text)
	// The scan may finish after the add; give a late scan a moment and drain.
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case <-e.sess.Events():
		case <-deadline:
			goto drained
		}
	}
drained:
	rows := e.sess.Snapshot().Rows
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.InstallDir]++
	}
	if counts[custom] != 1 {
		t.Fatalf("rows for added dir = %d, want 1: %+v", counts[custom], rows)
	}
	if counts[e.gameRoot] != 1 {
		t.Fatalf("rows for steam game = %d, want 1: %+v", counts[e.gameRoot], rows)
	}
	loaded, err := settings.Load(e.sess.deps.SettingsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ExtraDirs) != 1 || loaded.ExtraDirs[0] != custom {
		t.Fatalf("persisted ExtraDirs: %v", loaded.ExtraDirs)
	}
	t.Logf("final rows consistent: %v", counts)
}

// TestRemoveDirectoryDuringInFlightAdd: removing a directory while its
// AddDirectory enrichment is still in flight must not leave a zombie row
// behind: the in-flight add observes the removal, never re-appends the row,
// never persists it to games.json, and never emits "directory added".
func TestRemoveDirectoryDuringInFlightAdd(t *testing.T) {
	e := newSlowCoversEnv(t, 2*time.Second)
	e.sess.deps.SettingsRoot = t.TempDir()
	custom := filepath.Join(t.TempDir(), "ZombieGame")
	writeUIFile(t, filepath.Join(custom, "game.exe"), "GAME")
	root := canonicalDir(custom)

	e.sess.AddDirectory(custom)
	e.sess.RemoveDirectory(custom)

	// The in-flight add must settle (its ctx cancelled) without hanging.
	deadline := time.Now().Add(10 * time.Second)
	for e.sess.OpBusy(root) {
		if time.Now().After(deadline) {
			t.Fatal("in-flight add never settled after RemoveDirectory")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Drain events: no "directory added" may arrive for the removed dir.
	drain := time.After(300 * time.Millisecond)
draining:
	for {
		select {
		case ev := <-e.sess.Events():
			t.Logf("event: %v %q", ev.Kind, ev.Text)
			if ev.Kind == EvScanDone && ev.Text == "directory added" {
				t.Fatal("removed directory still emitted 'directory added'")
			}
		case <-drain:
			break draining
		}
	}

	for _, r := range e.sess.Snapshot().Rows {
		if r.InstallDir == root {
			t.Fatalf("zombie row resurrected after RemoveDirectory: %+v", r)
		}
	}
	data, err := os.ReadFile(filepath.Join(e.sess.deps.SettingsRoot, "games.json"))
	if err != nil {
		t.Fatalf("games.json: %v", err)
	}
	if strings.Contains(string(data), root) {
		t.Errorf("games.json still contains removed dir %s", root)
	}
	t.Log("in-flight add observed the removal: no row, no cache entry, no event")
}

// TestClearBundleCache_Async: ClearBundleCache returns while the deletion
// is still in flight; the directory disappears shortly after and a
// completion toast is posted.
func TestClearBundleCache_Async(t *testing.T) {
	e := newTestEnv(t)
	dir := filepath.Join(e.sess.deps.CacheDir, "optiscaler", "v1")
	writeUIFile(t, filepath.Join(dir, "bundle.7z"), "cached")
	e.sess.removeAll = func(path string) error {
		time.Sleep(time.Second)
		return os.RemoveAll(path)
	}

	start := time.Now()
	e.sess.ClearBundleCache()
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("ClearBundleCache blocked %v (deletion takes 1s)", elapsed)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cache dir still exists after async clear")
		}
		time.Sleep(20 * time.Millisecond)
	}
	found := false
	for _, toast := range e.sess.Snapshot().Toasts {
		if toast.Text == "OptiScaler cache cleared" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing completion toast: %+v", e.sess.Snapshot().Toasts)
	}
	t.Log("clear returned immediately; dir deleted and toast posted async")
}
