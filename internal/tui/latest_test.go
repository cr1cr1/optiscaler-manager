package tui

import (
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

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// TestTUIVersionCycleLatestInstallsResolvedTag pins the non-absorbed Latest
// pick: Game One is committed at v0.10.0-test while LatestKnown() is
// v0.9.4-test (the fake GH serves the list head as "latest"), so 'v' lands
// on a leading Latest row and confirming it dispatches the literal "latest"
// — resolved at pick time by the session core, landing committed at the
// resolved tag (GUI dropdown parity).
func TestTUIVersionCycleLatestInstallsResolvedTag(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) {
		d.Settings = settings.Settings{DefaultVersion: "v0.10.0-test", OnlineLookups: true}
	})
	installed := installCommitted(t, e)
	if installed != "v0.10.0-test" {
		t.Fatalf("installed version = %q, want v0.10.0-test", installed)
	}
	pollUntil(t, "latest known", func() bool {
		return e.sess.LatestKnown() == "v0.9.4-test"
	})
	drainEvents(e.sess)

	m := New(e.sess, "test")
	m = pressRunes(m, "v")
	if m.cycle == nil {
		t.Fatal("'v' on an installed row did not stage a version switch")
	}
	// Exact cycle shape (GUI versionMenuRows parity): the concrete entry
	// semver-equal to the latest tag is REPLACED by the single Latest row,
	// never duplicated — [current, Latest (tag)] here, and the staged
	// candidate (post-advance) is that Latest row.
	if len(m.cycle.items) != 2 {
		t.Fatalf("cycle items = %+v, want exactly [current, Latest (tag)]", m.cycle.items)
	}
	cand := m.cycle.items[m.cycle.idx]
	if cand.ID != "latest" {
		t.Fatalf("first staged candidate ID = %q, want latest", cand.ID)
	}
	if cand.Label != "Latest (v0.9.4-test)" {
		t.Errorf("staged Latest label = %q, want %q", cand.Label, "Latest (v0.9.4-test)")
	}
	// The games-table cell renders the staged candidate (truncated to its
	// 15-column width — the uniform policy for long version strings).
	if frame := m.View(); !strings.Contains(frame, "→ Latest") {
		t.Errorf("staged-cycle frame does not render the Latest candidate:\n%s", frame)
	}
	t.Logf("staged-cycle frame (Latest candidate):\n%s", m.View())

	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.cycle != nil {
		t.Error("Enter did not clear the staged Latest pick")
	}
	pollUntil(t, "switch to the resolved latest tag", func() bool {
		rows := e.sess.Snapshot().Rows
		return len(rows) == 1 && rows[0].Status == domain.StatusCommitted &&
			rows[0].OptiScalerVersion == "v0.9.4-test"
	})
	t.Log("Latest pick resolved at pick time and installed v0.9.4-test")
}

// TestTUIVersionCycleLatestAbsorbed pins the absorbed case: Game One is
// committed AT the latest version (v0.9.4-test), so the cycle carries ONE
// Latest row in the current version's position (no duplicate concrete tag),
// and confirming it dispatches nothing (the S13 no-op, GUI parity).
func TestTUIVersionCycleLatestAbsorbed(t *testing.T) {
	e, installed := pinnedSwitchEnv(t)
	if e.sess.LatestKnown() != installed {
		t.Fatalf("LatestKnown = %q, want the installed %q for the absorbed case", e.sess.LatestKnown(), installed)
	}
	drainEvents(e.sess)

	m := New(e.sess, "test")
	m = pressRunes(m, "v") // stages the next candidate (v0.10.0-test)
	m = pressRunes(m, "v") // wraps back to the current version = the Latest row
	if m.cycle == nil {
		t.Fatal("staging vanished after two 'v' presses")
	}
	cand := m.cycle.items[m.cycle.idx]
	if cand.ID != "latest" {
		t.Errorf("wrapped candidate ID = %q, want latest (absorbed row)", cand.ID)
	}
	if cand.Label != "Latest (v0.9.4-test)" {
		t.Errorf("wrapped candidate label = %q, want %q", cand.Label, "Latest (v0.9.4-test)")
	}
	// Exact cycle shape: the Latest row REPLACED the current version's
	// concrete entry in place — [next, Latest (tag)], never a duplicate.
	if len(m.cycle.items) != 2 {
		t.Errorf("cycle items = %+v, want exactly [next, Latest (tag)]", m.cycle.items)
	}
	if m.cycle.items[1].ID != "latest" {
		t.Errorf("absorbed Latest row at index 1 = %+v, want the in-place row", m.cycle.items[1])
	}
	t.Logf("absorbed staged-cycle frame:\n%s", m.View())

	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.cycle != nil {
		t.Error("Enter did not clear the staged Latest pick")
	}
	select {
	case ev := <-e.sess.Events():
		t.Fatalf("confirming the absorbed Latest row dispatched %v %q, want silence", ev.Kind, ev.Text)
	case <-time.After(300 * time.Millisecond):
	}
	if e.sess.OpBusy(e.gameRoot) {
		t.Error("OpBusy true after a same-version Latest confirm: an op was registered")
	}
	if got := e.sess.Snapshot().Rows[0].OptiScalerVersion; got != installed {
		t.Errorf("version after absorbed-Latest confirm = %q, want %q", got, installed)
	}
	t.Log("absorbed Latest row confirmed: no dispatch, version unchanged")
}

// TestTUIVersionCycleSingleVersionAbsorbedNoOp pins the <2 rule under
// absorption: a game whose only known version IS the latest yields a
// one-row cycle (the absorbed Latest row) — staging is a no-op, exactly
// as it was before the Latest option existed.
func TestTUIVersionCycleSingleVersionAbsorbedNoOp(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) {
		d.Settings = settings.Settings{DefaultVersion: "v0.9.4-test", OnlineLookups: true}
	})
	installed := installCommitted(t, e)
	if e.sess.LatestKnown() != installed {
		t.Fatalf("LatestKnown = %q, want the installed %q", e.sess.LatestKnown(), installed)
	}
	drainEvents(e.sess)

	m := New(e.sess, "test")
	m = pressRunes(m, "v")
	if m.cycle != nil {
		t.Errorf("'v' staged a cycle %+v on a single-version game, want a no-op", m.cycle.items)
	}
	select {
	case ev := <-e.sess.Events():
		t.Fatalf("single-version 'v' dispatched %v %q, want silence", ev.Kind, ev.Text)
	case <-time.After(300 * time.Millisecond):
	}
	t.Log("single-version game at latest: 'v' stays a no-op")
}

// TestTUIVersionCycleConcreteWrapNoOp pins the CONCRETE branch of the S13
// no-op: installed v0.10.0-test (not the latest), the cycle wraps from the
// Latest row back onto the current concrete entry, and confirming it
// dispatches nothing (the model suppresses the same-version confirm even
// when the Latest row exists).
func TestTUIVersionCycleConcreteWrapNoOp(t *testing.T) {
	e := newTestEnv(t, func(d *ui.Deps) {
		d.Settings = settings.Settings{DefaultVersion: "v0.10.0-test", OnlineLookups: true}
	})
	installed := installCommitted(t, e)
	pollUntil(t, "latest known", func() bool {
		return e.sess.LatestKnown() == "v0.9.4-test"
	})
	drainEvents(e.sess)

	m := New(e.sess, "test")
	m = pressRunes(m, "v") // stages the Latest row (next after current)
	m = pressRunes(m, "v") // wraps onto the current concrete entry
	if m.cycle == nil {
		t.Fatal("staging vanished after two 'v' presses")
	}
	if cand := m.cycle.items[m.cycle.idx]; cand.ID != installed {
		t.Fatalf("wrapped candidate = %q, want the concrete current %q", cand.ID, installed)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	select {
	case ev := <-e.sess.Events():
		t.Fatalf("concrete same-version confirm dispatched %v %q, want silence", ev.Kind, ev.Text)
	case <-time.After(300 * time.Millisecond):
	}
	if e.sess.OpBusy(e.gameRoot) {
		t.Error("OpBusy true after a concrete same-version confirm")
	}
	if got := e.sess.Snapshot().Rows[0].OptiScalerVersion; got != installed {
		t.Errorf("version after concrete wrap confirm = %q, want %q", got, installed)
	}
	t.Log("concrete wrap-to-current confirm dispatched nothing")
}

// latestPickEnv is newTestEnv with one difference the pick-time pin needs:
// the fake GH's release payload is MUTABLE (the fixture is duplicated —
// ponytail: the price of a payload newTestEnv cannot swap). Returns the
// env, the payload swapper, and the cache dir (the test removes
// cooldown.json to force the pick-time resolve live).
func latestPickEnv(t *testing.T) (*testEnv, func(head string), string) {
	t.Helper()
	e := &testEnv{}

	var mu sync.Mutex
	head := "v0.9.4-test"
	bundleURL := "/bundle" // replaced with the absolute URL once the server is up
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		h, bu := head, bundleURL
		mu.Unlock()
		fmt.Fprintf(w, `[{"tag_name":%q,"prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]},{"tag_name":"v0.10.0-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]}]`, h, bu, bu)
	})
	mux.HandleFunc("/bundle", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("..", "installer", "testdata", "bundle.7z"))
	})
	e.srv = httptest.NewServer(mux)
	t.Cleanup(e.srv.Close)
	mu.Lock()
	bundleURL = e.srv.URL + "/bundle"
	mu.Unlock()

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

	ghClient := gh.NewWithBaseURL(nil, filepath.Join(root, "cache"), e.srv.URL)
	deps := ui.Deps{
		Store:        store.New(root),
		GH:           ghClient,
		Covers:       covers.NewWithBase(nil, filepath.Join(root, "covers"), e.srv.URL+"/cdn/%s", e.srv.URL+"/search/"),
		CacheDir:     filepath.Join(root, "cache"),
		SteamRoot:    steamRoot,
		SettingsRoot: filepath.Join(root, "settings"),
		Settings:     settings.Settings{DefaultVersion: "v0.10.0-test", OnlineLookups: true},
	}
	e.sess = ui.NewSession(deps)

	swap := func(newHead string) {
		mu.Lock()
		defer mu.Unlock()
		head = newHead
	}
	return e, swap, filepath.Join(root, "cache")
}

// TestTUIVersionCycleLatestResolvesAtPickTime pins the user-facing promise
// end-to-end through the TUI: the fake GH publishes a NEWER release AFTER
// the startup memo was taken, the clock is pushed past the cooldown so the
// pick-time resolve must go live, and confirming the staged Latest row
// installs the NEW head — the literal "latest" dispatched by the model is
// re-resolved at pick time, never the stale startup memo (which
// LatestKnown keeps, startup-only by design).
func TestTUIVersionCycleLatestResolvesAtPickTime(t *testing.T) {
	e, swap, cacheDir := latestPickEnv(t)
	installed := installCommitted(t, e)
	if installed != "v0.10.0-test" {
		t.Fatalf("installed version = %q, want v0.10.0-test", installed)
	}
	pollUntil(t, "latest memo taken", func() bool {
		return e.sess.LatestKnown() == "v0.9.4-test"
	})
	drainEvents(e.sess)

	m := New(e.sess, "test")
	m = pressRunes(m, "v")
	if m.cycle == nil {
		t.Fatal("'v' did not stage a version switch")
	}
	if cand := m.cycle.items[m.cycle.idx]; cand.ID != "latest" {
		t.Fatalf("staged candidate = %q, want the Latest row", cand.ID)
	}

	// A newer stable appears after the memo. Drop the cooldown marker so
	// the pick-time resolve cannot serve the install-written cache and
	// must go live against the flipped payload.
	swap("v0.9.5-test")
	if err := os.Remove(filepath.Join(cacheDir, "cooldown.json")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove cooldown.json: %v", err)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	pollUntil(t, "install of the pick-time-resolved head", func() bool {
		rows := e.sess.Snapshot().Rows
		return len(rows) == 1 && rows[0].Status == domain.StatusCommitted &&
			rows[0].OptiScalerVersion == "v0.9.5-test"
	})
	if got := e.sess.LatestKnown(); got != "v0.9.4-test" {
		t.Errorf("LatestKnown changed to %q mid-session, want the startup memo v0.9.4-test", got)
	}
	t.Log("Latest pick re-resolved at pick time: installed the newer v0.9.5-test, memo unchanged")
}
