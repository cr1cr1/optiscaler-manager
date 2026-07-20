package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// gatedCoversEnv wires a Session whose first cover CDN request parks on a
// gate the test controls, so one scan can be held mid-flight while any
// (illegally concurrent) later scan's requests pass instantly.
type gatedCoversEnv struct {
	*testEnv
	blocked chan struct{} // closed when the first CDN request arrives
	gate    chan struct{} // close to release the parked request
}

func newGatedCoversEnv(t *testing.T) *gatedCoversEnv {
	t.Helper()
	e := &testEnv{}
	g := &gatedCoversEnv{testEnv: e, blocked: make(chan struct{}), gate: make(chan struct{})}
	var first atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/cdn/", func(w http.ResponseWriter, r *http.Request) {
		if first.Add(1) == 1 {
			close(g.blocked)
			<-g.gate
		}
		w.WriteHeader(http.StatusNotFound)
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

	e.sess = NewSession(Deps{
		Store:     store.New(root),
		Covers:    covers.NewWithBase(nil, filepath.Join(root, "covers"), e.srv.URL+"/cdn/%s", e.srv.URL+"/search/"),
		CacheDir:  filepath.Join(root, "cache"),
		SteamRoot: e.steamRoot,
	})
	return g
}

// drainScanEvents collects events until none arrives for idleWindow and
// returns the counts of scan starts and scan completions (add-directory
// completions excluded).
func drainScanEvents(s *Session, idleWindow time.Duration) (starts, dones int) {
	timer := time.NewTimer(idleWindow)
	defer timer.Stop()
	for {
		select {
		case ev := <-s.Events():
			if ev.Kind == EvScanStarted {
				starts++
			}
			if ev.Kind == EvScanDone && ev.Text != "directory added" {
				dones++
			}
			timer.Reset(idleWindow)
		case <-timer.C:
			return starts, dones
		}
	}
}

// TestScanSerialization_ContainerAddDuringBootScan: a container added while
// a scan is parked mid-flight must not lose its children. The losing
// interleaving (unguarded Scan): rescan B surfaces the children and settles
// first; the parked boot scan A settles last and its keep-block preserves
// only rows whose InstallDir is itself an extra dir — the children are
// wiped until the next manual rescan. Serialized scans re-run after A with
// the post-add snapshot, so the children survive.
func TestScanSerialization_ContainerAddDuringBootScan(t *testing.T) {
	g := newGatedCoversEnv(t)
	e := g.testEnv
	e.sess.deps.SettingsRoot = t.TempDir()

	container := filepath.Join(t.TempDir(), "Library")
	childA := filepath.Join(container, "ChildA")
	childB := filepath.Join(container, "ChildB")
	writeUIFile(t, filepath.Join(childA, "bin", "game.exe"), "GAME")
	writeUIFile(t, filepath.Join(childB, "bin", "game.exe"), "GAME")

	// Park the boot scan on its first cover fetch.
	e.sess.Scan(context.Background())
	select {
	case <-g.blocked:
	case <-time.After(10 * time.Second):
		t.Fatal("boot scan never reached its first cover fetch")
	}

	// Adding a container mid-scan registers the scan root and requests a
	// rescan; an unguarded rescan runs concurrently and settles first.
	e.sess.AddDirectory(container)
	drainScanEvents(e.sess, 2*time.Second)

	// Release the parked boot scan and let every scan settle.
	close(g.gate)
	drainScanEvents(e.sess, time.Second)

	found := map[string]bool{}
	for _, r := range e.sess.Snapshot().Rows {
		found[r.InstallDir] = true
	}
	if !found[childA] || !found[childB] {
		t.Fatalf("container children wiped by the parked scan's late settle: rows %+v",
			e.sess.Snapshot().Rows)
	}
	t.Log("container children survived the add-during-boot-scan interleaving")
}

// TestScanSerialization_PendingRunsOnce: Scan calls landing while a scan is
// in flight coalesce into a single follow-up scan — two Scan calls during
// one parked scan produce exactly one re-run (two starts total), not one
// goroutine per call.
func TestScanSerialization_PendingRunsOnce(t *testing.T) {
	g := newGatedCoversEnv(t)
	e := g.testEnv
	e.sess.deps.SettingsRoot = t.TempDir()

	e.sess.Scan(context.Background())
	select {
	case <-g.blocked:
	case <-time.After(10 * time.Second):
		t.Fatal("scan never reached its first cover fetch")
	}

	e.sess.Scan(context.Background())
	e.sess.Scan(context.Background())
	close(g.gate)

	starts, dones := drainScanEvents(e.sess, time.Second)
	if starts != 2 || dones != 2 {
		t.Fatalf("scan starts/dones = %d/%d, want 2/2 (parked scan + one coalesced re-run)",
			starts, dones)
	}
	t.Log("two mid-scan Scan calls coalesced into exactly one follow-up scan")
}
