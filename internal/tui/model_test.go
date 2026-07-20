package tui

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/launch"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
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
	sess      *ui.Session
	gameRoot  string
	bin       string
	steamRoot string
	srv       *httptest.Server
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
	e.steamRoot = steamRoot
	e.gameRoot = filepath.Join(steamRoot, "steamapps", "common", "GameOne")
	e.bin = filepath.Join(e.gameRoot, "bin")
	writeFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+steamRoot+`" } }`)
	writeFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`)
	writeFile(t, filepath.Join(e.bin, "gameone.exe"), "GAME")
	writeFile(t, filepath.Join(e.bin, "nvngx_dlss.dll"), "DLSS")

	deps := ui.Deps{
		Store:        store.New(root),
		GH:           gh.NewWithBaseURL(nil, filepath.Join(root, "cache"), e.srv.URL),
		Covers:       covers.NewWithBase(nil, filepath.Join(root, "covers"), e.srv.URL+"/cdn/%s", e.srv.URL+"/search/"),
		CacheDir:     filepath.Join(root, "cache"),
		SteamRoot:    steamRoot,
		SettingsRoot: filepath.Join(root, "settings"),
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
	return startTUISize(t, sess, 100, 30)
}

func startTUISize(t *testing.T, sess *ui.Session, w, h int) *teatest.TestModel {
	t.Helper()
	tm := teatest.NewTestModel(t, New(sess, "v0.0.0-test"), teatest.WithInitialTermSize(w, h))
	t.Cleanup(func() { _ = tm.Quit() })
	return tm
}

// seedGamesCache writes a games-list cache (games.json) into root, mirroring
// internal/ui's cache schema so Session.Start boots warm without scanning.
func seedGamesCache(t *testing.T, root string, rows []ui.GameRow) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(struct {
		Version int          `json:"version"`
		Rows    []ui.GameRow `json:"rows"`
	}{Version: 3, Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "games.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func pollUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
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

// TestTUIInstallRoundTrip presses i on the selected row: the session op
// runs end-to-end (files on disk) and the status line reports the install.
func TestTUIInstallRoundTrip(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("i")
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

// TestDetailKeyOpenINIAllowedForExternal: pressing o on the detail screen of
// an external row (CanOpenINI true) reaches the opener seam — a fake
// xdg-open earlier in PATH records every path handed to the platform opener;
// a never-installed row stays gated off.
func TestDetailKeyOpenINIAllowedForExternal(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "opened.log")
	xdg := filepath.Join(binDir, "xdg-open")
	writeFile(t, xdg, "#!/bin/sh\nprintf '%s\n' \"$1\" >> \"$XDG_OPEN_LOG\"\n")
	if err := os.Chmod(xdg, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_OPEN_LOG", logPath)

	opened := func() string {
		data, err := os.ReadFile(logPath)
		if err != nil {
			return ""
		}
		return string(data)
	}
	oKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}

	externalDetail := func(t *testing.T, status domain.Status, withINI bool) (Model, string) {
		t.Helper()
		settingsDir := t.TempDir()
		e := newTestEnv(t, func(d *ui.Deps) { d.SettingsRoot = settingsDir })
		iniDir := ""
		if withINI {
			iniDir = e.bin
			writeFile(t, filepath.Join(e.bin, "OptiScaler.ini"), "[Upscalers]\n")
		}
		seedGamesCache(t, settingsDir, []ui.GameRow{{
			Title:        "Game One",
			AppID:        "100",
			InstallDir:   e.gameRoot,
			InjectionDir: iniDir,
			Platform:     "Steam",
			Status:       status,
		}})
		e.sess.Start(context.Background())
		return Model{sess: e.sess, screen: screenDetail, detailDir: e.gameRoot}, e.bin
	}

	t.Run("external row opens", func(t *testing.T) {
		m, bin := externalDetail(t, domain.StatusExternal, true)
		m.detailKey(oKey)
		pollUntil(t, "opener seam invoked for an external row", func() bool {
			return strings.Contains(opened(), filepath.Join(bin, "OptiScaler.ini"))
		})
	})

	t.Run("never-installed row stays gated", func(t *testing.T) {
		m, _ := externalDetail(t, "", false)
		before := opened()
		m.detailKey(oKey)
		time.Sleep(100 * time.Millisecond)
		if got := opened(); got != before {
			t.Errorf("opener seam invoked for a never-installed row; opened log %q", got)
		}
	})
}

// TestTUIConfirmEACPrompt: installing an EAC game surfaces the confirm
// modal; accepting it with 'y' lets the install proceed.
func TestTUIConfirmEACPrompt(t *testing.T) {
	e := newTestEnv(t, nil)
	writeFile(t, filepath.Join(e.gameRoot, "start_protected_game.exe"), "EAC")
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("i")
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
	if strings.Contains(frame, "[y] proceed") {
		t.Errorf("confirm modal still rendered after answering:\n%s", frame)
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

// TestTUIStartCachedShowsCachedRowsNoScan boots with a warm games cache: the
// cached rows render instantly, the status line reports "(cached)", and no
// scan ever runs (the fixture game must NOT appear).
func TestTUIStartCachedShowsCachedRowsNoScan(t *testing.T) {
	settingsDir := t.TempDir()
	e := newTestEnv(t, func(d *ui.Deps) { d.SettingsRoot = settingsDir })
	seedGamesCache(t, settingsDir, []ui.GameRow{{
		Title:      "Cached Phantom Game",
		AppID:      "999",
		InstallDir: "/phantom/dir",
		Platform:   "Steam",
	}})
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Cached Phantom Game")

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("cached-boot frame:\n%s", frame)
	for _, want := range []string{"Cached Phantom Game", "(cached)"} {
		if !strings.Contains(frame, want) {
			t.Errorf("cached-boot frame lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "Scanning") {
		t.Errorf("cache hit must never scan, frame shows scanning:\n%s", frame)
	}
	if strings.Contains(frame, "Game One") {
		t.Errorf("cache hit must not scan the fixture library:\n%s", frame)
	}
}

// TestTUITabSwitch: number keys switch between the Games, Settings, and Help
// screens.
func TestTUITabSwitch(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("2")
	waitFrame(t, tm, "Scan directories")
	tm.Type("3")
	waitFrame(t, tm, "Keyboard reference")
	tm.Type("1")

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("frame after switching back to games:\n%s", frame)
	for _, want := range []string{"Game One", "TITLE"} {
		if !strings.Contains(frame, want) {
			t.Errorf("games screen not restored after tab switch, lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "Keyboard reference") {
		t.Errorf("help screen still rendered after switching back:\n%s", frame)
	}
}

// TestTUISettingsListsDirectories: ExtraDirs from settings render in the
// settings screen's scan-directories section.
func TestTUISettingsListsDirectories(t *testing.T) {
	extra := t.TempDir()
	e := newTestEnv(t, func(d *ui.Deps) {
		d.Settings.ExtraDirs = []string{extra}
	})
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("2")
	waitFrame(t, tm, extra)

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("settings frame:\n%s", frame)
	for _, want := range []string{"Scan directories", extra, "Launch template", "Default OptiScaler version"} {
		if !strings.Contains(frame, want) {
			t.Errorf("settings frame lacks %q:\n%s", want, frame)
		}
	}
}

// TestTUISettingsRemoveDirectory: d on a listed directory asks for inline
// confirmation; y removes it from settings and from the rendered list.
func TestTUISettingsRemoveDirectory(t *testing.T) {
	extra := t.TempDir()
	e := newTestEnv(t, func(d *ui.Deps) {
		d.Settings.ExtraDirs = []string{extra}
	})
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("2")
	waitFrame(t, tm, extra)
	tm.Type("d")
	waitFrame(t, tm, "[y/n]")
	tm.Type("y")

	pollUntil(t, "ExtraDirs to shrink", func() bool {
		return len(e.sess.Settings().ExtraDirs) == 0
	})

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("settings frame after removal:\n%s", frame)
	if strings.Contains(frame, extra) {
		t.Errorf("removed directory still rendered:\n%s", frame)
	}
}

// TestTUISettingsAddDirectory: a opens a path input; entering an existing
// directory registers it through the session (settings + rendered list).
func TestTUISettingsAddDirectory(t *testing.T) {
	newDir := filepath.Join(t.TempDir(), "AddedGame")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A real exe makes the dir a game (v0.7): empty dirs are refused.
	if err := os.WriteFile(filepath.Join(newDir, "game.exe"), []byte("GAME"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("2")
	waitFrame(t, tm, "Scan directories")
	tm.Type("a")
	waitFrame(t, tm, "add dir:")
	tm.Type(newDir)
	sendKey(tm, tea.KeyEnter)

	pollUntil(t, "ExtraDirs to contain the new dir", func() bool {
		for _, d := range e.sess.Settings().ExtraDirs {
			if strings.Contains(d, "AddedGame") {
				return true
			}
		}
		return false
	})

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("settings frame after add:\n%s", frame)
	if !strings.Contains(frame, "AddedGame") {
		t.Errorf("added directory not rendered:\n%s", frame)
	}
}

// TestCommitInputAddDirDeferred: committing the add-dir input must not run
// AddDirectory on the update loop (session classification is synchronous) —
// commitInput returns a tea.Cmd the runtime executes off the loop.
func TestCommitInputAddDirDeferred(t *testing.T) {
	e := newTestEnv(t, nil)
	m := New(e.sess, "test")
	m.openInput(inputAddDir, "add dir: ", "")
	m.input.SetValue(filepath.Join(t.TempDir(), "missing"))

	cmd := m.commitInput()
	if cmd == nil {
		t.Fatal("commitInput returned nil cmd for a non-empty add-dir input")
	}
	if got := len(e.sess.Snapshot().Toasts); got != 0 {
		t.Fatalf("AddDirectory ran inline on the update loop: %d toasts", got)
	}

	cmd() // what the bubbletea runtime does with the returned command
	deadline := time.Now().Add(5 * time.Second)
	for len(e.sess.Snapshot().Toasts) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(e.sess.Snapshot().Toasts) == 0 {
		t.Fatal("deferred AddDirectory never toasted the missing path")
	}
	t.Log("add-dir commit deferred into a tea.Cmd")
}

// TestTUISettingsTemplateEdit: t opens the launch-template editor prefilled
// with the current value; Enter persists through the session setter.
func TestTUISettingsTemplateEdit(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("2")
	waitFrame(t, tm, "Launch template")
	tm.Type("t")
	waitFrame(t, tm, "launch template:")
	tm.Type(" --fast")
	sendKey(tm, tea.KeyEnter)

	pollUntil(t, "launch template update", func() bool {
		return e.sess.Settings().LaunchTemplate == `"{exe}" {args} --fast`
	})

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("settings frame after template edit:\n%s", frame)
	if !strings.Contains(frame, "--fast") {
		t.Errorf("edited template not rendered:\n%s", frame)
	}
}

// TestTUIDetailViewActions: enter on a row opens the detail screen with the
// game's metadata (path, AppID, versions) and the full actions list.
func TestTUIDetailViewActions(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	sendKey(tm, tea.KeyEnter)
	waitFrame(t, tm, "AppID")

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("detail frame:\n%s", frame)
	for _, want := range []string{"Game One", e.gameRoot, "100", "OptiScaler", "Actions", "rollback", "open INI", "launch"} {
		if !strings.Contains(frame, want) {
			t.Errorf("detail frame lacks %q:\n%s", want, frame)
		}
	}
}

// TestTUIConfirmModal: an EAC install gates behind a modal that swallows
// unrelated keys until answered; y lets the install proceed.
func TestTUIConfirmModal(t *testing.T) {
	e := newTestEnv(t, nil)
	writeFile(t, filepath.Join(e.gameRoot, "start_protected_game.exe"), "EAC")
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("i")
	waitFrame(t, tm, "[y] proceed")

	// Keys unrelated to the modal must be swallowed while it is up.
	sendKey(tm, tea.KeyDown)
	tm.Type("2jx")
	time.Sleep(200 * time.Millisecond)
	if e.sess.Snapshot().Confirm == nil {
		t.Fatal("unrelated keys answered or dismissed the confirm modal")
	}
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); !os.IsNotExist(err) {
		t.Fatal("install proceeded before the modal was answered")
	}

	tm.Type("y")
	waitFrame(t, tm, "Installed Game One")
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Fatalf("install did not proceed after accepting the modal: %v", err)
	}

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("frame after modal consent:\n%s", frame)
	if strings.Contains(frame, "[y] proceed") {
		t.Errorf("modal still rendered after answering:\n%s", frame)
	}
}

// TestTUIResizeKeepsCursorVisible: at a small terminal height, moving the
// cursor deep into a long list keeps the selected row rendered.
func TestTUIResizeKeepsCursorVisible(t *testing.T) {
	settingsDir := t.TempDir()
	e := newTestEnv(t, func(d *ui.Deps) { d.SettingsRoot = settingsDir })
	rows := make([]ui.GameRow, 0, 30)
	for i := 1; i <= 30; i++ {
		rows = append(rows, ui.GameRow{
			Title:      fmt.Sprintf("Game %02d", i),
			AppID:      fmt.Sprintf("%d", 1000+i),
			InstallDir: fmt.Sprintf("/phantom/%02d", i),
			Platform:   "Steam",
		})
	}
	seedGamesCache(t, settingsDir, rows)
	tm := startTUISize(t, e.sess, 100, 12)

	waitFrame(t, tm, "Game 01")
	tm.Type(strings.Repeat("j", 25))
	waitFrame(t, tm, "Game 26")

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("small-height scrolled frame:\n%s", frame)
	if !strings.Contains(frame, "Game 26") {
		t.Errorf("selected row scrolled out of view:\n%s", frame)
	}
}

// TestTUIQuitsOnCtrlC: ctrl+c terminates the program from the games screen.
func TestTUIQuitsOnCtrlC(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	sendKey(tm, tea.KeyCtrlC)
	tm.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
	t.Log("quit cleanly on ctrl+c")
}

// TestTUIEmptyLibraryGuidance: an empty library renders friendly guidance
// instead of a blank screen.
func TestTUIEmptyLibraryGuidance(t *testing.T) {
	e := newTestEnv(t, nil)
	if err := os.Remove(filepath.Join(e.steamRoot, "steamapps", "appmanifest_100.acf")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(e.steamRoot, "steamapps", "common")); err != nil {
		t.Fatal(err)
	}
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "no games yet")

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("empty-library frame:\n%s", frame)
	if !strings.Contains(frame, "no games yet") {
		t.Errorf("empty-state guidance missing:\n%s", frame)
	}
}

// TestTUIFooterShowsScreenHintsOnGames: the games footer advertises the
// number-key screen switches (2 settings, 3 help, 4 about); without them the
// tab bar was the only discoverability cue and users never found Settings.
func TestTUIFooterShowsScreenHintsOnGames(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Game One")) &&
			bytes.Contains(b, []byte("2 settings")) &&
			bytes.Contains(b, []byte("3 help")) &&
			bytes.Contains(b, []byte("4 about"))
	}, teatest.WithDuration(15*time.Second), teatest.WithCheckInterval(10*time.Millisecond))

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("games frame with footer hints:\n%s", frame)
	for _, want := range []string{"2 settings", "3 help", "4 about"} {
		if !strings.Contains(frame, want) {
			t.Errorf("games footer lacks screen hint %q:\n%s", want, frame)
		}
	}
}

// TestTUIAboutScreen: key 4 opens the About screen, which shows the injected
// version and the TUI stack line (answers "are you using bubbletea?" on
// screen).
func TestTUIAboutScreen(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("4")
	waitFrame(t, tm, "v0.0.0-test")

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("about frame:\n%s", frame)
	for _, want := range []string{"optiscaler-manager v0.0.0-test", "bubbletea"} {
		if !strings.Contains(frame, want) {
			t.Errorf("about frame lacks %q:\n%s", want, frame)
		}
	}
}

// TestTUIAboutInTabBarAndHelp: the tab bar carries the "4 About" entry and
// the Help screen's Global line lists the 4 about binding.
func TestTUIAboutInTabBarAndHelp(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "4 About")
	tm.Type("3")
	waitFrame(t, tm, "Keyboard reference")

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("help frame:\n%s", frame)
	if !strings.Contains(frame, "4 about") {
		t.Errorf("help Global line lacks the about binding:\n%s", frame)
	}
}

// TestTUIInputModeShowsEscHint: every text-input mode renders an escape hint
// next to the input so users are never trapped (filter and settings add-dir
// shown; the other editors share the same footer path).
func TestTUIInputModeShowsEscHint(t *testing.T) {
	e := newTestEnv(t, nil)
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("/")
	waitFrame(t, tm, "esc cancel")
	sendKey(tm, tea.KeyEsc)

	tm.Type("2")
	waitFrame(t, tm, "Scan directories")
	tm.Type("a")
	waitFrame(t, tm, "add dir:")

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("add-dir input frame:\n%s", frame)
	if !strings.Contains(frame, "esc cancel") {
		t.Errorf("input mode lacks the escape hint:\n%s", frame)
	}
}

// TestTUITierDisplay: a row carrying a ProtonDB tier shows it in the games
// table (as a badge) and in the detail view, alongside the Steam AppID.
func TestTUITierDisplay(t *testing.T) {
	settingsDir := t.TempDir()
	e := newTestEnv(t, func(d *ui.Deps) { d.SettingsRoot = settingsDir })
	seedGamesCache(t, settingsDir, []ui.GameRow{{
		Title:      "Tiered Game",
		AppID:      "555",
		InstallDir: "/phantom/tiered",
		Platform:   "Steam",
		SteamAppID: "555",
		ProtonTier: "gold",
	}})
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "gold")
	sendKey(tm, tea.KeyEnter)
	waitFrame(t, tm, "ProtonDB")

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("tiered detail frame:\n%s", frame)
	for _, want := range []string{"gold", "ProtonDB", "Steam AppID"} {
		if !strings.Contains(frame, want) {
			t.Errorf("tiered detail frame lacks %q:\n%s", want, frame)
		}
	}
}

// TestTUIOnlineLookupsToggle: the settings screen shows the online-lookups
// state and o toggles it through the session.
func TestTUIOnlineLookupsToggle(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) { d.Settings = settings.Defaults() })
	tm := startTUI(t, e.sess)

	waitFrame(t, tm, "Game One")
	tm.Type("2")
	waitFrame(t, tm, "online game info: on")
	tm.Type("o")

	pollUntil(t, "OnlineLookups toggled off", func() bool {
		return !e.sess.Settings().OnlineLookups
	})

	_ = tm.Quit()
	frame := finalFrame(t, tm)
	t.Logf("settings frame after toggle:\n%s", frame)
	if !strings.Contains(frame, "online game info: off") {
		t.Errorf("settings frame does not show the toggled state:\n%s", frame)
	}
}

// TestNewCarriesVersionIntoModel: tui.New plumbs the build version into the
// model so the About screen can render it.
func TestNewCarriesVersionIntoModel(t *testing.T) {
	e := newTestEnv(t, nil)
	m := New(e.sess, "v9.9.9-test")
	if m.version != "v9.9.9-test" {
		t.Errorf("Model.version = %q, want %q", m.version, "v9.9.9-test")
	}
}
