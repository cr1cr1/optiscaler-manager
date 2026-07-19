package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/launch"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestMain silences zerolog: session ops log through the global logger,
// which would otherwise spam the test output.
func TestMain(m *testing.M) {
	log.Logger = zerolog.Nop()
	os.Exit(m.Run())
}

// testEnv wires a ui.Session against fakes (httptest GitHub + CDN, temp
// store, temp Steam root with one game) — the same seams the ui tests use,
// expressed through the exported API only.
type testEnv struct {
	sess     *ui.Session
	gameRoot string
	bin      string
	srv      *httptest.Server
}

func newTestEnv(t *testing.T, mutate func(*ui.Deps)) *testEnv {
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
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/search/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	e.srv = httptest.NewServer(mux)
	t.Cleanup(e.srv.Close)

	root := t.TempDir()
	steamRoot := t.TempDir()
	e.gameRoot = filepath.Join(steamRoot, "steamapps", "common", "GameOne")
	e.bin = filepath.Join(e.gameRoot, "bin")
	writeFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+steamRoot+`" } }`)
	writeFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`)
	writeFile(t, filepath.Join(e.bin, "gameone.exe"), "GAME")
	writeFile(t, filepath.Join(e.bin, "nvngx_dlss.dll"), "DLSS")

	deps := ui.Deps{
		Store:     store.New(root),
		GH:        gh.NewWithBaseURL(nil, filepath.Join(root, "cache"), e.srv.URL),
		Covers:    covers.NewWithBase(nil, filepath.Join(root, "covers"), e.srv.URL+"/cdn/%s", e.srv.URL+"/search/"),
		CacheDir:  filepath.Join(root, "cache"),
		SteamRoot: steamRoot,
	}
	if mutate != nil {
		mutate(&deps)
	}
	e.sess = ui.NewSession(deps)
	return e
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// startTUI runs the model under teatest with a deterministic terminal size.
func startTUI(t *testing.T, sess *ui.Session) *teatest.TestModel {
	t.Helper()
	tm := teatest.NewTestModel(t, New(sess), teatest.WithInitialTermSize(100, 30))
	t.Cleanup(func() { _ = tm.Quit() })
	return tm
}

// waitFrame blocks until the rendered output contains want.
func waitFrame(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte(want))
	}, teatest.WithDuration(15*time.Second), teatest.WithCheckInterval(10*time.Millisecond))
}

func sendKey(tm *teatest.TestModel, k tea.KeyType) {
	tm.Send(tea.KeyMsg{Type: k})
}

// finalFrame quits the program and returns the final model's rendered view.
func finalFrame(t *testing.T, tm *teatest.TestModel) string {
	t.Helper()
	m, ok := tm.FinalModel(t, teatest.WithFinalTimeout(10*time.Second)).(Model)
	if !ok {
		t.Fatalf("final model type %T, want tui.Model", tm.FinalModel(t))
	}
	return m.View()
}

// TestTUIListsGames scans the fixture library and asserts the rendered frame
// carries title, store, tech badge, and the status line (golden-ish frame
// assertion on the final view, plus the captured frame as a log artifact).
func TestTUIListsGames(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	// One WaitFor per synchronization point: WaitFor consumes the output
	// stream, so back-to-back waits on the same frame lose bytes.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Game One")) && bytes.Contains(b, []byte("1 games"))
	}, teatest.WithDuration(15*time.Second), teatest.WithCheckInterval(10*time.Millisecond))

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("captured frame:\n%s", frame)
	for _, want := range []string{"Game One", "Steam", "DLSS", "1 games"} {
		if !strings.Contains(frame, want) {
			t.Errorf("final frame lacks %q:\n%s", want, frame)
		}
	}
}

// TestTUIFilter enters filter mode with '/', narrows the list to nothing,
// then Esc clears the filter and the row returns.
func TestTUIFilter(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("/zzz")
	waitFrame(t, tm, "(no matches)")

	sendKey(tm, tea.KeyEsc)
	deadline := time.Now().Add(5 * time.Second)
	for e.sess.Snapshot().Query != "" {
		if time.Now().After(deadline) {
			t.Fatalf("Esc did not clear the session query (still %q)", e.sess.Snapshot().Query)
		}
		time.Sleep(10 * time.Millisecond)
	}

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("frame after clearing filter:\n%s", frame)
	if !strings.Contains(frame, "Game One") {
		t.Errorf("row did not return after clearing the filter:\n%s", frame)
	}
	if strings.Contains(frame, "(no matches)") {
		t.Errorf("empty-list marker survived clearing the filter:\n%s", frame)
	}
}

// TestTUIInstallRoundTrip presses enter on the selected row: the session op
// runs end-to-end (files on disk) and the status line reports the install.
func TestTUIInstallRoundTrip(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	sendKey(tm, tea.KeyEnter)
	waitFrame(t, tm, "Installed Game One")

	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Fatalf("keypress-driven install did not land files: %v", err)
	}

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("frame after install:\n%s", frame)
	if !strings.Contains(frame, "Installed Game One") {
		t.Errorf("status line does not reflect the install:\n%s", frame)
	}
}

// TestTUIQuitsOnQ: q terminates the program.
func TestTUIQuitsOnQ(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))

	m, ok := tm.FinalModel(t).(Model)
	if !ok {
		t.Fatalf("final model type %T, want tui.Model", tm.FinalModel(t))
	}
	t.Logf("quit cleanly; final status line %q", m.View())
}

// TestTUIConfirmEACPrompt: installing an EAC game surfaces the confirm
// dialog; accepting it with 'y' lets the install proceed.
func TestTUIConfirmEACPrompt(t *testing.T) {
	e := newTestEnv(t, nil)
	writeFile(t, filepath.Join(e.gameRoot, "start_protected_game.exe"), "EAC")
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	sendKey(tm, tea.KeyEnter)
	waitFrame(t, tm, "Easy Anti-Cheat")
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); !os.IsNotExist(err) {
		t.Fatal("install proceeded before the EAC prompt was answered")
	}

	tm.Type("y")
	waitFrame(t, tm, "Installed Game One")
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Fatalf("install did not proceed after accepting the EAC prompt: %v", err)
	}

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("frame after EAC consent:\n%s", frame)
	if strings.Contains(frame, "[y/N]") {
		t.Errorf("confirm dialog still rendered after answering:\n%s", frame)
	}
}

// launchCapture records the argv handed to the injected runner.
type launchCapture struct {
	mu   sync.Mutex
	name string
	args []string
}

func (c *launchCapture) runner() launch.Runner {
	return func(_ context.Context, dir, name string, args ...string) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.name, c.args = name, append([]string(nil), args...)
		return nil
	}
}

func (c *launchCapture) argv() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.name == "" {
		return nil
	}
	return append([]string{c.name}, c.args...)
}

func noBinaries(string) (string, error) { return "", errors.New("not found") }

// TestTUILaunchBinding presses 'l' on the selected row and asserts the
// session's Launch path fired through the injected launcher seam.
func TestTUILaunchBinding(t *testing.T) {
	cap := &launchCapture{}
	e := newTestEnv(t, func(d *ui.Deps) {
		d.Launcher = launch.New(cap.runner(), "linux", noBinaries)
	})
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("l")
	waitFrame(t, tm, "Launch requested")

	argv := cap.argv()
	found := false
	for _, a := range argv {
		if strings.Contains(a, "steam://rungameid/100") {
			found = true
		}
	}
	if !found {
		t.Fatalf("captured argv %v lacks steam://rungameid/100", argv)
	}
	t.Logf("captured launch argv: %v", argv)

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("frame after launch:\n%s", frame)
	if !strings.Contains(frame, "Launch requested: Game One") {
		t.Errorf("status line does not reflect the launch:\n%s", frame)
	}
}
