package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
