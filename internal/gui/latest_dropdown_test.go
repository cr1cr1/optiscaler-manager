package gui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// latestDropdownEnv builds the user-state session for the "latest" option
// tests: the default version preference is "latest" with online lookups
// on, and the latest release (v0.9.4-test) already in the download cache.
// The row is installed by the caller (committed at the resolved tag via
// QuickInstall, or external at a planted PE version).
func latestDropdownEnv(t *testing.T) (*ui.Session, string) {
	t.Helper()
	sess, gameRoot := guiFakes(t, func(d *ui.Deps) {
		d.Settings = settings.Defaults() // default "latest", online lookups on
		// The REAL test bundle (a valid 7z): the committed-row helper
		// quick-installs from this cached copy, so it must be extractable.
		bundle, err := os.ReadFile(filepath.Join("..", "installer", "testdata", "bundle.7z"))
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(d.CacheDir, "optiscaler", "v0.9.4-test")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Optiscaler_test.7z"), bundle, 0o644); err != nil {
			t.Fatal(err)
		}
	})
	return sess, gameRoot
}

// waitRow polls until the session's row meets want (a version-free row for
// the clean-scan stage, a versioned row after install/enrichment).
func waitRow(t *testing.T, sess *ui.Session, want func(ui.GameRow) bool) ui.GameRow {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for _, r := range sess.VisibleRows() {
			if want(r) {
				return r
			}
		}
		select {
		case <-sess.Events():
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("row never reached the wanted state")
	return ui.GameRow{}
}

// committedAtLatest boots the session through Start (cold: scan + the
// startup latest preload) and quick-installs the game at the resolved
// default ("latest" → v0.9.4-test, served from the pre-cached bundle),
// returning the committed row — the user's own row shape (installed at the
// latest, tag form).
func committedAtLatest(t *testing.T) (*ui.Session, ui.GameRow) {
	t.Helper()
	sess, gameRoot := latestDropdownEnv(t)
	sess.Start(context.Background())
	waitRow(t, sess, func(r ui.GameRow) bool { return r.InstallDir != "" })
	sess.QuickInstall(gameRoot)
	row := waitRow(t, sess, func(r ui.GameRow) bool { return r.OptiScalerVersion != "" })
	if row.Status != domain.StatusCommitted {
		t.Fatalf("row status = %q, want committed", row.Status)
	}
	return sess, row
}

// TestVersionDropdownMenuOffersLatest (user spec, bug report): "`latest` …
// is not present in the optiscaler dropdown menu as an option. There are
// only old/cached versions." Repro state mirrors the user's machine: the
// default version preference is "latest" with online lookups on, the
// latest release (v0.9.4-test) already cached, and the row installed at
// that same latest version — so the OLD menu was exactly "one old/cached
// version" and nothing the user could read as "latest".
//
// Expected: the opened menu offers `latest` as an option — a row whose
// rendered label reads as latest. (Written red first: no such row existed
// — the literal "latest" was deliberately never offered; only the concrete
// tag, indistinguishable from a stale cached one.)
func TestVersionDropdownMenuOffersLatest(t *testing.T) {
	sess, row := committedAtLatest(t)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 400, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	openDropdown(t, m, row.InstallDir, view)

	var labels []string
	for _, it := range m.versionDDItems {
		labels = append(labels, it.label)
	}
	for _, l := range labels {
		if strings.Contains(strings.ToLower(l), "latest") {
			t.Logf("menu offers latest: %v", labels)
			return
		}
	}
	t.Fatalf("menu offers no 'latest' option (rows %v): only old/cached versions", labels)
}

// TestVersionDropdown_LatestRowAbsorbedSingleTick: when the installed
// version IS the latest, the menu shows ONE row — the concrete entry
// absorbed into the "Latest (…)" label — and exactly one tick (the
// absorbed row, ticked as the current version).
func TestVersionDropdown_LatestRowAbsorbedSingleTick(t *testing.T) {
	sess, row := committedAtLatest(t)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 400, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	openDropdown(t, m, row.InstallDir, view)

	if len(m.versionDDItems) != 1 {
		t.Fatalf("menu rows %d, want 1 (the latest entry absorbs the concrete one): %+v", len(m.versionDDItems), m.versionDDItems)
	}
	it := m.versionDDItems[0]
	if !strings.Contains(strings.ToLower(it.label), "latest") {
		t.Errorf("single row label %q does not read as latest", it.label)
	}
	if !it.ticked {
		t.Error("absorbed latest row not ticked against the installed current version")
	}
	t.Logf("absorbed latest row: %q ticked", it.label)
}

// TestVersionDropdown_LatestRowDispatchesLatest: on a row installed BELOW
// the latest, the Latest row sits at the top, is not ticked, and
// dispatches SwitchVersion(dir, "latest") — the literal, so the switch
// re-resolves at pick time — while the concrete rows keep dispatching
// their own tags.
func TestVersionDropdown_LatestRowDispatchesLatest(t *testing.T) {
	sess, gameRoot := latestDropdownEnv(t)
	// The row is external at an OLDER planted version (0.9.3), below the
	// latest (v0.9.4-test), so the latest row is a distinct, non-ticked
	// top entry.
	markExternal(t, filepath.Join(gameRoot, "bin"), [4]uint16{0, 9, 3, 0})
	sess.Start(context.Background())
	row := waitRow(t, sess, func(r ui.GameRow) bool { return r.OptiScalerVersion != "" })
	m := newModel(Config{Session: sess})
	type call struct{ dir, version string }
	var calls []call
	m.switchVersionFn = func(dir, version string) { calls = append(calls, call{dir, version}) }

	headlessFrames(t, 400, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	openDropdown(t, m, row.InstallDir, view)

	latestIdx := -1
	for i, it := range m.versionDDItems {
		if strings.Contains(strings.ToLower(it.label), "latest") {
			latestIdx = i
		}
	}
	if latestIdx < 0 {
		t.Fatalf("no latest row: %+v", m.versionDDItems)
	}
	if latestIdx != 0 {
		t.Errorf("latest row at index %d, want 0 (latest is the maximum)", latestIdx)
	}
	if m.versionDDItems[latestIdx].ticked {
		t.Error("latest row ticked although the installed version is older")
	}
	clickRect(m.versionDDItems[latestIdx].rect, view)
	keyFrame(KeyCodeNone, 0, view)

	if len(calls) != 1 {
		t.Fatalf("SwitchVersion dispatches %d, want 1", len(calls))
	}
	if calls[0].dir != row.InstallDir || calls[0].version != "latest" {
		t.Errorf("SwitchVersion(%q, %q), want (%q, %q)", calls[0].dir, calls[0].version, row.InstallDir, "latest")
	}
	if m.versionDDItemsFor != "" {
		t.Errorf("dropdown still open after the latest pick (owner %q)", m.versionDDItemsFor)
	}
	t.Log("picking the latest row dispatched the literal \"latest\"")
}
