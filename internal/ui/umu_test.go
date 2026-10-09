package ui

import (
	"slices"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/umu"
)

var testPrefixes = []umu.Prefix{
	{Name: "SomeApp", Path: "/home/me/Games/Heroic/Prefixes/SomeApp", Source: "heroic"},
	{Name: "mybottle", Path: "/home/me/.local/share/bottles/bottles/mybottle", Source: "bottles"},
}

// TestUmuPickerRowsComposition: the shared picker rows are the Auto row
// first (with the caller's label), then the detected prefixes in scan
// order, plus a Custom row when the pinned path matches no detected
// prefix. Rows carry Source and Name separately so the frontends can
// render "icon + Launcher: prefix" (issue 059).
func TestUmuPickerRowsComposition(t *testing.T) {
	labels := func(rows []UmuPickerOption) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = UmuPickerRowLabel(r)
		}
		return out
	}

	auto := UmuPickerRows("Auto (detected)", "", testPrefixes)
	if got, want := labels(auto), []string{"Auto (detected)", "Heroic: SomeApp", "Bottles: mybottle"}; !slices.Equal(got, want) {
		t.Errorf("rows(empty) = %v, want %v", got, want)
	}
	if auto[0].Value != "" || auto[0].Source != "" {
		t.Errorf("Auto row = %+v, want empty value and source", auto[0])
	}
	if auto[1].Source != "heroic" || auto[1].Name != "SomeApp" {
		t.Errorf("rows[1] = %+v, want Source heroic / Name SomeApp", auto[1])
	}

	known := UmuPickerRows("Auto (detected)", testPrefixes[1].Path, testPrefixes)
	if len(known) != 3 {
		t.Errorf("rows(known pin) = %d rows, want 3 (no Custom row)", len(known))
	}

	custom := UmuPickerRows("Auto (inherit)", "/elsewhere/prefix", testPrefixes)
	last := custom[len(custom)-1]
	if last.Value != "/elsewhere/prefix" || last.Source != "custom" || UmuPickerRowLabel(last) != "Custom: /elsewhere/prefix" {
		t.Errorf("Custom row = %+v, want value/source/label for /elsewhere/prefix", last)
	}
	if custom[0].Name != "Auto (inherit)" {
		t.Errorf("Auto row name = %q, want the caller's %q", custom[0].Name, "Auto (inherit)")
	}

	if got := UmuPickerLabel("Auto (detected)", "", testPrefixes); got != "Auto (detected)" {
		t.Errorf("label(empty) = %q, want Auto (detected)", got)
	}
	if got := UmuPickerLabel("Auto (detected)", testPrefixes[0].Path, testPrefixes); got != "Heroic: SomeApp" {
		t.Errorf("label(known) = %q, want Heroic: SomeApp", got)
	}
	if got := UmuPickerLabel("Auto (detected)", "/elsewhere/prefix", testPrefixes); got != "Custom: /elsewhere/prefix" {
		t.Errorf("label(unknown) = %q, want Custom: /elsewhere/prefix", got)
	}

	names := map[string]string{
		"steam": "Steam", "bottles": "Bottles", "lutris": "Lutris",
		"heroic": "Heroic", "playonlinux": "PlayOnLinux",
		"crossover": "CrossOver", "wine": "Wine", "custom": "Custom",
	}
	for source, want := range names {
		if got := UmuSourceName(source); got != want {
			t.Errorf("UmuSourceName(%q) = %q, want %q", source, got, want)
		}
	}
}

// TestSessionUmuPrefixes_UsesInjectedProvider: the settings frontends
// read the picker entries from the injected deps provider verbatim.
func TestSessionUmuPrefixes_UsesInjectedProvider(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.UmuPrefixes = func() []umu.Prefix { return testPrefixes }

	got := e.sess.UmuPrefixes()
	if len(got) != len(testPrefixes) {
		t.Fatalf("len(UmuPrefixes()) = %d, want %d", len(got), len(testPrefixes))
	}
	for i, p := range testPrefixes {
		if got[i] != p {
			t.Errorf("UmuPrefixes()[%d] = %+v, want %+v", i, got[i], p)
		}
	}
}

// TestSessionUmuPrefixes_NilProviderFallsBackToScan: without an injected
// provider the production scan runs; on an empty HOME it finds nothing.
func TestSessionUmuPrefixes_NilProviderFallsBackToScan(t *testing.T) {
	e := newTestEnv(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	if got := e.sess.UmuPrefixes(); len(got) != 0 {
		t.Errorf("UmuPrefixes() = %v, want none on an empty HOME", got)
	}
}

// TestSessionSetUmuPrefixPath: the global default prefix persists and
// an empty value returns to Auto.
func TestSessionSetUmuPrefixPath(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	e.sess.SetUmuPrefixPath(testPrefixes[0].Path)
	if got := e.sess.Settings().UmuPrefixPath; got != testPrefixes[0].Path {
		t.Errorf("UmuPrefixPath = %q, want %q", got, testPrefixes[0].Path)
	}
	s, err := settings.Load(e.sess.deps.SettingsRoot)
	if err != nil {
		t.Fatalf("settings unreadable: %v", err)
	}
	if s.UmuPrefixPath != testPrefixes[0].Path {
		t.Errorf("persisted UmuPrefixPath = %q, want %q", s.UmuPrefixPath, testPrefixes[0].Path)
	}

	e.sess.SetUmuPrefixPath("")
	if got := e.sess.Settings().UmuPrefixPath; got != "" {
		t.Errorf("UmuPrefixPath = %q after clear, want empty (Auto)", got)
	}
}

// TestSessionUmuPrefixOverride: a per-game pin persists in
// PrefixOverrides (canonical dir key), wins over the global default in
// UmuPrefixFor, and an empty value clears it.
func TestSessionUmuPrefixOverride(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	e.sess.Scan(t.Context())
	waitScanDone(t, e.sess)
	dir := e.gameRoot
	e.sess.SetUmuPrefixPath("/global/prefix")

	e.sess.SetUmuPrefixOverride(dir, testPrefixes[1].Path)
	if got := e.sess.Settings().PrefixOverrides[canonicalDir(dir)]; got != testPrefixes[1].Path {
		t.Errorf("PrefixOverrides[%s] = %q, want %q", canonicalDir(dir), got, testPrefixes[1].Path)
	}
	if got := e.sess.UmuPrefixFor(dir); got != testPrefixes[1].Path {
		t.Errorf("UmuPrefixFor = %q, want per-game pin %q", got, testPrefixes[1].Path)
	}
	if got := e.sess.UmuPrefixPin(dir); got != testPrefixes[1].Path {
		t.Errorf("UmuPrefixPin = %q, want %q", got, testPrefixes[1].Path)
	}
	s, err := settings.Load(e.sess.deps.SettingsRoot)
	if err != nil {
		t.Fatalf("settings unreadable: %v", err)
	}
	if s.PrefixOverrides[canonicalDir(dir)] != testPrefixes[1].Path {
		t.Errorf("persisted PrefixOverrides = %v, want the pin", s.PrefixOverrides)
	}

	// Clearing the pin falls back to the global default.
	e.sess.SetUmuPrefixOverride(dir, "")
	if got := e.sess.UmuPrefixFor(dir); got != "/global/prefix" {
		t.Errorf("UmuPrefixFor after clear = %q, want global default /global/prefix", got)
	}
	if _, ok := e.sess.Settings().PrefixOverrides[canonicalDir(dir)]; ok {
		t.Errorf("PrefixOverrides still holds %s after clear", canonicalDir(dir))
	}
}

// TestSessionUmuPrefixFor_NoPins: with neither pin set the effective
// prefix is empty (the launch hook runs the auto chain).
func TestSessionUmuPrefixFor_NoPins(t *testing.T) {
	e := newTestEnv(t)
	if got := e.sess.UmuPrefixFor(e.gameRoot); got != "" {
		t.Errorf("UmuPrefixFor = %q, want empty with no pins", got)
	}
	if got := e.sess.UmuPrefixPin(e.gameRoot); got != "" {
		t.Errorf("UmuPrefixPin = %q, want empty with no per-game pin", got)
	}
}

// TestSessionUmuPrefixOverride_UnknownGame: pinning for a dir the
// session does not know toasts a warning and persists nothing.
func TestSessionUmuPrefixOverride_UnknownGame(t *testing.T) {
	e := newTestEnv(t)
	e.sess.SetUmuPrefixOverride("/no/such/game", testPrefixes[0].Path)
	if len(e.sess.Settings().PrefixOverrides) != 0 {
		t.Errorf("PrefixOverrides = %v for unknown game, want nothing persisted", e.sess.Settings().PrefixOverrides)
	}
}
