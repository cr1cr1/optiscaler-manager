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
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// TestSessionCancelOp_AbortsAndCleans cancels an in-flight install via
// CancelOp: the op must abort, the row must return to its pre-op status,
// exactly one "Cancelled" toast/event must surface (no failure spam), and
// neither the game dir nor the bundle cache may hold partial state.
func TestSessionCancelOp_AbortsAndCleans(t *testing.T) {
	e := &testEnv{}

	downloading := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.9.4-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]}]`, e.srv.URL+"/bundle")
	})
	mux.HandleFunc("/bundle", func(w http.ResponseWriter, r *http.Request) {
		close(downloading)
		<-r.Context().Done() // hang until the op's context dies
	})
	mux.HandleFunc("/cdn/", func(w http.ResponseWriter, r *http.Request) {
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
	writeUIFile(t, filepath.Join(e.bin, "nvngx_dlss.dll"), "DLSS")
	cacheDir := filepath.Join(root, "cache")

	e.sess = NewSession(Deps{
		Store:     store.New(root),
		GH:        gh.NewWithBaseURL(nil, cacheDir, e.srv.URL),
		Covers:    covers.NewWithBase(nil, filepath.Join(root, "covers"), e.srv.URL+"/cdn/%s", e.srv.URL+"/search/"),
		CacheDir:  cacheDir,
		SteamRoot: e.steamRoot,
	})

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	e.sess.Install(e.gameRoot)
	select {
	case <-downloading:
	case <-time.After(15 * time.Second):
		t.Fatal("download never started; op not in flight")
	}
	if !e.sess.CancelOp(e.gameRoot) {
		t.Fatal("CancelOp reported no in-flight op during download")
	}

	// Drain until the cancel settles; no failure event may appear.
	failedSeen := false
	deadline := time.After(15 * time.Second)
	for {
		select {
		case ev := <-e.sess.Events():
			t.Logf("event: %v %q", ev.Kind, ev.Text)
			if ev.Kind == EvOpFailed {
				failedSeen = true
			}
			if ev.Kind == EvOpCancelled {
				goto settled
			}
		case <-deadline:
			t.Fatal("timed out waiting for EvOpCancelled")
		}
	}
settled:
	if failedSeen {
		t.Error("EvOpFailed emitted for a cancelled op (error spam)")
	}

	st := e.sess.Snapshot()
	cancelToasts := 0
	for _, toast := range st.Toasts {
		t.Logf("toast: %q warn=%v", toast.Text, toast.Warn)
		if strings.Contains(toast.Text, "Cancelled") {
			cancelToasts++
		}
		if toast.Warn {
			t.Errorf("warning toast for a clean cancel: %q", toast.Text)
		}
	}
	if cancelToasts != 1 {
		t.Errorf("cancel toasts = %d, want exactly 1", cancelToasts)
	}
	if st.Busy != "" {
		t.Errorf("Busy %q after cancel", st.Busy)
	}

	// Row back to its pre-op status (never installed).
	for _, r := range st.Rows {
		if r.InstallDir == e.gameRoot && r.Status != "" {
			t.Errorf("row status %q after cancel, want pre-op %q", r.Status, domain.Status(""))
		}
	}

	// Game dir holds exactly its original files.
	entries, err := os.ReadDir(e.bin)
	if err != nil {
		t.Fatalf("read bin: %v", err)
	}
	var names []string
	for _, en := range entries {
		names = append(names, en.Name())
	}
	if len(names) != 2 {
		t.Errorf("bin after cancel: %v, want [gameone.exe nvngx_dlss.dll]", names)
	}
	for _, n := range names {
		if n != "gameone.exe" && n != "nvngx_dlss.dll" {
			t.Errorf("foreign file in game dir after cancel: %s", n)
		}
	}

	// No manifest, no partial bundle in the versioned cache.
	manifests, lerr := e.sess.deps.Store.List()
	if lerr != nil || len(manifests) != 0 {
		t.Errorf("manifests after cancel: %d (%v), want 0", len(manifests), lerr)
	}
	matches, _ := filepath.Glob(filepath.Join(cacheDir, "optiscaler", "*", "*"))
	for _, m := range matches {
		st_, serr := os.Stat(m)
		if serr == nil && !st_.IsDir() {
			t.Errorf("partial bundle left in cache: %s", m)
		}
	}

	// The op slot is released.
	if e.sess.CancelOp(e.gameRoot) {
		t.Error("CancelOp succeeded after the op settled; slot not released")
	}
	t.Log("cancelled op: aborted, one toast, row restored, FS+cache clean, slot released")
}
