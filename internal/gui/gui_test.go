package gui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// guiFakes builds a session against the same fakes as the ui package tests.
// Optional dep mutators let a test inject seams (e.g. a capturing launcher).
func guiFakes(t *testing.T, opts ...func(*ui.Deps)) (*ui.Session, string) {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.9.4-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]}]`, srv.URL+"/bundle")
	})
	mux.HandleFunc("/bundle", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("..", "installer", "testdata", "bundle.7z"))
	})
	mux.HandleFunc("/cdn/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/search/", func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `{"items":[]}`) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	root := t.TempDir()
	steamRoot := t.TempDir()
	gameRoot := filepath.Join(steamRoot, "steamapps", "common", "GameOne")
	bin := filepath.Join(gameRoot, "bin")
	writeGUIFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+steamRoot+`" } }`)
	writeGUIFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`)
	writeGUIFile(t, filepath.Join(bin, "gameone.exe"), "GAME")
	writeGUIFile(t, filepath.Join(bin, "nvngx_dlss.dll"), "DLSS")

	deps := ui.Deps{
		Store:        store.New(root),
		GH:           gh.NewWithBaseURL(nil, filepath.Join(root, "cache"), srv.URL),
		Covers:       covers.NewWithBase(nil, filepath.Join(root, "covers"), srv.URL+"/cdn/%s", srv.URL+"/search/"),
		CacheDir:     filepath.Join(root, "cache"),
		SteamRoot:    steamRoot,
		SettingsRoot: filepath.Join(root, "settings"), // realistic default; mutators may override
	}
	for _, o := range opts {
		o(&deps)
	}
	sess := ui.NewSession(deps)
	return sess, gameRoot
}

func writeGUIFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGUIBindsSessionState(t *testing.T) {
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})

	sess.Scan(context.Background())
	deadline := time.Now().Add(15 * time.Second)
	for m.state.StatusLine != "1 games" && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
			m.drain()
		case <-time.After(20 * time.Millisecond):
		}
	}
	if len(m.state.Rows) != 1 {
		t.Fatalf("model state not bound: %+v", m.state)
	}
	if m.state.Rows[0].Title != "Game One" {
		t.Errorf("row title %q", m.state.Rows[0].Title)
	}
	t.Logf("bound state: %s, %d rows", m.state.StatusLine, len(m.state.Rows))
}

func TestGUIFilterSyncsToSession(t *testing.T) {
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})
	m.filter = "cyber"
	m.syncFilter()
	if got := sess.Snapshot().Query; got != "cyber" {
		t.Errorf("session query %q, want cyber", got)
	}
}

func TestEscClosesModal(t *testing.T) {
	sess, gameRoot := guiFakes(t)
	writeGUIFile(t, filepath.Join(gameRoot, "start_protected_game.exe"), "EAC")
	m := newModel(Config{Session: sess})

	sess.Scan(context.Background())
	deadline := time.Now().Add(15 * time.Second)
	for len(sess.VisibleRows()) == 0 && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	sess.QuickInstall(gameRoot) // EAC gate: stops at the confirmation modal
	for m.state.Confirm == nil && time.Now().Before(deadline) {
		select {
		case <-sess.Events():
			m.drain()
		case <-time.After(20 * time.Millisecond):
		}
	}
	if m.state.Confirm == nil {
		t.Fatal("EAC install did not raise the confirmation modal")
	}

	headlessFrames(t, 1000, 700)
	keyFrame(KeyCodeNone, 0, m.rootView) // open frame
	keyFrame(KeyEscape, 0, m.rootView)   // universal close gesture
	m.drain()
	if got := sess.Snapshot().Confirm; got != nil {
		t.Errorf("confirm modal still pending after Esc: %+v", got)
	}
	t.Log("Esc dismissed the EAC confirm modal")
}

func TestRenderToPNGSmoke(t *testing.T) {
	m := newModel(Config{AuditGrid: false})
	m.state = ui.State{
		StatusLine: "2 games",
		Mode:       ui.ViewGrid,
		Rows: []ui.GameRow{
			{Title: "Cyberpunk 2077", AppID: "1091500", Status: domain.StatusCommitted,
				TechBadges: []ui.Badge{{Label: "DLSS", Tone: ui.ToneGreen}}, CoverPath: ""},
			{Title: "Broken Game", AppID: "42", Status: domain.StatusFailed, Actionable: true},
		},
		Toasts: []ui.Toast{{Text: "Installed Cyberpunk 2077", AddedAt: time.Now()}},
	}

	out := filepath.Join(t.TempDir(), "frame.png")
	if err := renderToPNG(out, 1000, 700, m.rootView); err != nil {
		t.Fatalf("renderToPNG: %v", err)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatalf("no PNG produced: %v", err)
	}
	if st.Size() == 0 {
		t.Fatal("PNG is empty")
	}
	t.Logf("smoke frame: %d bytes", st.Size())
}

func TestRenderToPNGSmokeWithConfirm(t *testing.T) {
	m := newModel(Config{})
	m.state = ui.State{
		StatusLine: "1 games",
		Rows:       []ui.GameRow{{Title: "Protected", AppID: "1", EAC: true}},
		Confirm:    &ui.Confirmation{Kind: ui.ConfirmEAC, GameDir: "/g/p", Message: "Protected uses Easy Anti-Cheat. Installing OptiScaler may result in a ban."},
	}
	out := filepath.Join(t.TempDir(), "frame.png")
	if err := renderToPNG(out, 1000, 700, m.rootView); err != nil {
		t.Fatalf("renderToPNG with confirm modal: %v", err)
	}
	if st, _ := os.Stat(out); st == nil || st.Size() == 0 {
		t.Fatal("empty PNG")
	}
	t.Log("confirm modal renders")
}
