//go:build linux

package optiscalermanager

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// fakeUmuRun installs a fake umu-run script on PATH: it answers
// --version like the real thing and otherwise records the PROTONPATH /
// WINEPREFIX / GAMEID / STORE environment into the file named by
// UMU_TEST_CAPTURE.
func fakeUmuRun(t *testing.T) (capture string) {
	t.Helper()
	bin := t.TempDir()
	capture = filepath.Join(t.TempDir(), "capture")
	script := "#!/bin/sh\n" +
		`if [ "$1" = "--version" ]; then echo "umu-launcher version 1.4.4 (Python 3.11.6)"; exit 0; fi` + "\n" +
		`printf '%s\n%s\n%s\n%s\n' "$PROTONPATH" "$WINEPREFIX" "$GAMEID" "$STORE" > "$UMU_TEST_CAPTURE"` + "\n"
	if err := os.WriteFile(filepath.Join(bin, "umu-run"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake umu-run: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("UMU_TEST_CAPTURE", capture)
	return capture
}

func readCapture(t *testing.T, capture string) (proton, prefix string) {
	proton, prefix, _, _ = readCaptureAll(t, capture)
	return proton, prefix
}

// readCaptureGame returns GAMEID and STORE.
func readCaptureGame(t *testing.T, capture string) (gameID, store string) {
	_, _, gameID, store = readCaptureAll(t, capture)
	return gameID, store
}

// readCaptureAll returns the four captured env values.
func readCaptureAll(t *testing.T, capture string) (proton, prefix, gameID, store string) {
	t.Helper()
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("capture = %q, want four lines (PROTONPATH, WINEPREFIX, GAMEID, STORE)", string(data))
	}
	return lines[0], lines[1], lines[2], lines[3]
}

// TestUmuLauncherResolvesGameIDStore: the launch-time gameIDFor
// resolver's result becomes the GAMEID/STORE env (issue 064) so
// protonfixes apply per-game fixes.
func TestUmuLauncherResolvesGameIDStore(t *testing.T) {
	capture := fakeUmuRun(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())

	hook := newUmuLauncher(
		func(string) string { return "" }, func(string) string { return "" }, func() bool { return true }, "",
		func(string) string { return "" },
		func(context.Context, ui.GameRow) (string, string) { return "umu-1174180", "egs" })
	row := ui.GameRow{
		Title:      "RDR2",
		InstallDir: t.TempDir(),
		ExePath:    filepath.Join(t.TempDir(), "rdr2.exe"),
	}
	if _, err := hook(context.Background(), row); err != nil {
		t.Fatalf("hook: %v", err)
	}
	gameID, store := readCaptureGame(t, capture)
	if gameID != "umu-1174180" || store != "egs" {
		t.Errorf("GAMEID/STORE = (%q, %q), want (umu-1174180, egs)", gameID, store)
	}
}

// TestUmuLauncherAutoDetectsProton: with the Proton path setting left
// empty the hook launches with PROTONPATH pointing at the newest
// detected runner (here: a Heroic-installed GE-Proton without a
// toolmanifest.vdf) and the app-managed prefix.
func TestUmuLauncherAutoDetectsProton(t *testing.T) {
	capture := fakeUmuRun(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	data := t.TempDir()
	t.Setenv("OM_DATA_DIR", data)

	runner := filepath.Join(home, ".config", "heroic", "tools", "proton", "GE-Proton10-1")
	if err := os.MkdirAll(runner, 0o755); err != nil {
		t.Fatalf("mkdir runner: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runner, "proton"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write proton script: %v", err)
	}

	hook := newUmuLauncher(func(string) string { return "" }, func(string) string { return "" }, func() bool { return true }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	if hook == nil {
		t.Fatal("newUmuLauncher returned nil")
	}
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: filepath.Join(t.TempDir(), "Foo"),
		ExePath:    filepath.Join(t.TempDir(), "Foo", "foo.exe"),
	}
	if _, err := hook(context.Background(), row); err != nil {
		t.Fatalf("hook: %v", err)
	}

	proton, prefix := readCapture(t, capture)
	if proton != runner {
		t.Errorf("PROTONPATH = %q, want auto-detected %q", proton, runner)
	}
	if !strings.HasPrefix(prefix, filepath.Join(data, "umu-prefixes")) {
		t.Errorf("WINEPREFIX = %q, want app-managed prefix under %q", prefix, data)
	}
}

// TestUmuLauncherReusesEnclosingPrefix: a game installed inside an
// existing bottle launches with WINEPREFIX pointed at that bottle.
func TestUmuLauncherReusesEnclosingPrefix(t *testing.T) {
	capture := fakeUmuRun(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())

	bottle := filepath.Join(t.TempDir(), "bottle")
	install := filepath.Join(bottle, "drive_c", "Games", "Foo")
	if err := os.MkdirAll(install, 0o755); err != nil {
		t.Fatalf("mkdir bottle: %v", err)
	}

	hook := newUmuLauncher(func(string) string { return "" }, func(string) string { return "" }, func() bool { return true }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	if hook == nil {
		t.Fatal("newUmuLauncher returned nil")
	}
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: install,
		ExePath:    filepath.Join(install, "foo.exe"),
	}
	if _, err := hook(context.Background(), row); err != nil {
		t.Fatalf("hook: %v", err)
	}

	proton, prefix := readCapture(t, capture)
	if proton != "" {
		t.Errorf("PROTONPATH = %q, want empty (no runners installed)", proton)
	}
	if prefix != bottle {
		t.Errorf("WINEPREFIX = %q, want enclosing bottle %q", prefix, bottle)
	}
}

// TestUmuLauncherPinnedRunnerWins: the live runner resolver's result
// becomes PROTONPATH verbatim (per-game pin > global default > auto —
// the precedence is the resolver's job, tested in internal/ui);
// a user-picked runner is never re-resolved (issue 063).
func TestUmuLauncherPinnedRunnerWins(t *testing.T) {
	capture := fakeUmuRun(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())

	// A detected Steam runner that would win Auto — the pin must beat it.
	autoRunner := filepath.Join(home, ".steam", "steam", "compatibilitytools.d", "UMU-Proton-99-1")
	if err := os.MkdirAll(autoRunner, 0o755); err != nil {
		t.Fatalf("mkdir runner: %v", err)
	}
	if err := os.WriteFile(filepath.Join(autoRunner, "proton"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write proton script: %v", err)
	}

	hook := newUmuLauncher(
		func(string) string { return "" }, func(string) string { return "" }, func() bool { return true }, "",
		func(string) string { return "/picked/GE-Proton9-20" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    filepath.Join(t.TempDir(), "foo.exe"),
	}
	if _, err := hook(context.Background(), row); err != nil {
		t.Fatalf("hook: %v", err)
	}
	proton, _ := readCapture(t, capture)
	if proton != "/picked/GE-Proton9-20" {
		t.Errorf("PROTONPATH = %q, want the pinned /picked/GE-Proton9-20 (not auto %q)", proton, autoRunner)
	}
}

// TestUmuLauncherPerGamePinWins: the live pin resolver's result becomes
// WINEPREFIX verbatim (per-game pin > global default > auto — the
// precedence is the resolver's job, tested in internal/ui).
func TestUmuLauncherPerGamePinWins(t *testing.T) {
	capture := fakeUmuRun(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())

	hook := newUmuLauncher(func(string) string { return "/pinned/prefix" }, func(string) string { return "umu" }, func() bool { return true }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	if hook == nil {
		t.Fatal("newUmuLauncher returned nil with umu-run on PATH")
	}
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    filepath.Join(t.TempDir(), "foo.exe"),
	}
	if _, err := hook(context.Background(), row); err != nil {
		t.Fatalf("hook: %v", err)
	}

	proton, prefix := readCapture(t, capture)
	if proton != "" {
		t.Errorf("PROTONPATH = %q, want empty (no runners installed)", proton)
	}
	if prefix != "/pinned/prefix" {
		t.Errorf("WINEPREFIX = %q, want the pinned /pinned/prefix", prefix)
	}
}

// fakeCLI installs a fake CLI on PATH recording its argv into the file
// named by CLI_TEST_CAPTURE (the launchercli test pattern, reused at
// this level to prove the router picks the owner backend).
func fakeCLI(t *testing.T, name string) (capture string) {
	t.Helper()
	bin := t.TempDir()
	capture = filepath.Join(t.TempDir(), "capture-"+name)
	script := "#!/bin/sh\n" +
		`printf '%s\n' "$@" > "$CLI_TEST_CAPTURE"` + "\n"
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CLI_TEST_CAPTURE", capture)
	return capture
}

// readCLIArgv polls for the detached fake CLI's argv capture.
func readCLIArgv(t *testing.T, capture string) []string {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); ; {
		data, err := os.ReadFile(capture)
		if err == nil && len(data) > 0 {
			return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		}
		if time.Now().After(deadline) {
			t.Fatalf("capture %s never written", capture)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestUmuRouteRejectsLauncherOwnedPrefix: a FORCED umu route with a
// launcher-owned prefix (here Bottles) is a launch error naming the
// owner and the remedy — umu mutates any Proton prefix it launches
// (DLL symlinks, pfx/ layout), so running it against a foreign prefix
// corrupts that launcher's bottle (issue 062). Auto never reaches this
// path: owned prefixes route to the owner CLI (issue 060).
func TestUmuRouteRejectsLauncherOwnedPrefix(t *testing.T) {
	umuCaptureFile := fakeUmuRun(t)
	_, bottle := bottlesFixture(t)

	hook := newUmuLauncher(func(string) string { return bottle }, func(string) string { return "umu" }, func() bool { return true }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    filepath.Join(t.TempDir(), "foo.exe"),
	}
	_, err := hook(context.Background(), row)
	if err == nil {
		t.Fatal("forced umu route with a Bottles prefix: want error, got nil")
	}
	for _, want := range []string{"bottles", "corrupt", "auto"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if _, statErr := os.Stat(umuCaptureFile); !os.IsNotExist(statErr) {
		t.Error("umu-run was invoked against a Bottles-owned prefix")
	}
}

// TestUmuRouteRejectsSteamCompatdata: Steam compatdata belongs to
// Steam — a forced umu route with a pinned compatdata prefix errors
// the same way (issue 062).
func TestUmuRouteRejectsSteamCompatdata(t *testing.T) {
	umuCaptureFile := fakeUmuRun(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())
	// A compatdata prefix with an appmanifest makes it Steam-owned.
	steamapps := filepath.Join(home, ".steam", "steam", "steamapps")
	pfx := filepath.Join(steamapps, "compatdata", "12345", "pfx")
	if err := os.MkdirAll(filepath.Join(pfx, "drive_c"), 0o755); err != nil {
		t.Fatalf("mkdir compatdata: %v", err)
	}
	manifest := filepath.Join(steamapps, "appmanifest_12345.acf")
	if err := os.WriteFile(manifest, []byte(`"AppState" { "appid" "12345" "name" "Foo Game" }`), 0o644); err != nil {
		t.Fatalf("write appmanifest: %v", err)
	}

	hook := newUmuLauncher(func(string) string { return pfx }, func(string) string { return "umu" }, func() bool { return true }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    filepath.Join(t.TempDir(), "foo.exe"),
	}
	_, err := hook(context.Background(), row)
	if err == nil {
		t.Fatal("forced umu route with Steam compatdata: want error, got nil")
	}
	if !strings.Contains(err.Error(), "steam") {
		t.Errorf("error %q lacks the owner name (steam)", err)
	}
	if _, statErr := os.Stat(umuCaptureFile); !os.IsNotExist(statErr) {
		t.Error("umu-run was invoked against Steam compatdata")
	}
}

// TestUmuRouteAllowsDotWine: ~/.wine has no launcher owner — a forced
// umu route may use it (issue 062).
func TestUmuRouteAllowsDotWine(t *testing.T) {
	capture := fakeUmuRun(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())
	dotwine := filepath.Join(home, ".wine")
	if err := os.MkdirAll(filepath.Join(dotwine, "drive_c"), 0o755); err != nil {
		t.Fatalf("mkdir ~/.wine: %v", err)
	}

	hook := newUmuLauncher(func(string) string { return dotwine }, func(string) string { return "umu" }, func() bool { return true }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    filepath.Join(t.TempDir(), "foo.exe"),
	}
	if _, err := hook(context.Background(), row); err != nil {
		t.Fatalf("forced umu route with ~/.wine: %v", err)
	}
	_, prefix := readCapture(t, capture)
	if prefix != dotwine {
		t.Errorf("WINEPREFIX = %q, want %q", prefix, dotwine)
	}
}

// bottlesFixture builds a HOME with a Bottles prefix and returns the
// prefix path.
func bottlesFixture(t *testing.T) (home, bottle string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())
	bottle = filepath.Join(home, ".local", "share", "bottles", "bottles", "mybottle")
	if err := os.MkdirAll(filepath.Join(bottle, "drive_c"), 0o755); err != nil {
		t.Fatalf("mkdir bottle: %v", err)
	}
	return home, bottle
}

// TestUmuRouterAutoRoutesToBottles: under auto, a game pinned to a
// Bottles-owned prefix launches via bottles-cli with the exact exe —
// umu-run is never invoked (issue 060).
func TestUmuRouterAutoRoutesToBottles(t *testing.T) {
	umuCaptureFile := fakeUmuRun(t)
	cliCapture := fakeCLI(t, "bottles-cli")
	_, bottle := bottlesFixture(t)

	hook := newUmuLauncher(func(string) string { return bottle }, func(string) string { return "" }, func() bool { return true }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	if hook == nil {
		t.Fatal("newUmuLauncher returned nil")
	}
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    "/games/foo/foo.exe",
	}
	via, err := hook(context.Background(), row)
	if err != nil {
		t.Fatalf("hook: %v", err)
	}
	if via != "bottles" {
		t.Errorf("via = %q, want bottles", via)
	}
	argv := readCLIArgv(t, cliCapture)
	if got, want := strings.Join(argv, " "), "run -b mybottle -e /games/foo/foo.exe"; got != want {
		t.Errorf("bottles-cli argv = %q, want %q", got, want)
	}
	if _, err := os.Stat(umuCaptureFile); !os.IsNotExist(err) {
		t.Error("umu-run was invoked for a Bottles-owned prefix")
	}
}

// TestUmuRouterAutoUnownedWithoutUmuErrors: umu toggle state is
// irrelevant to routing — with no owned prefix and no umu-run
// installed, auto fails with a naming error instead of raw-exec'ing
// the PE into binfmt/mono (issue 060).
func TestUmuRouterAutoUnownedWithoutUmuErrors(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no umu-run
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())

	hook := newUmuLauncher(func(string) string { return "" }, func(string) string { return "" }, func() bool { return true }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	if hook == nil {
		t.Fatal("newUmuLauncher returned nil — the router must exist without umu-run")
	}
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    filepath.Join(t.TempDir(), "foo.exe"),
	}
	_, err := hook(context.Background(), row)
	if err == nil || !strings.Contains(err.Error(), "umu-run") {
		t.Errorf("err = %v, want it naming umu-run", err)
	}
}

// TestUmuRouterUnownedUmuDisabledErrors: with the umu feature toggle
// off (umuAllowed false), an unowned prefix does not launch via umu
// even when umu-run is installed — the toggle gates the umu backend,
// never the owner route (issue 060).
func TestUmuRouterUnownedUmuDisabledErrors(t *testing.T) {
	fakeUmuRun(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())

	hook := newUmuLauncher(func(string) string { return "" }, func(string) string { return "" }, func() bool { return false }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    filepath.Join(t.TempDir(), "foo.exe"),
	}
	_, err := hook(context.Background(), row)
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Errorf("err = %v, want it naming the disabled umu-launcher", err)
	}
}

// TestUmuRouterOwnerForcedUnownedErrors: route "owner" with an
// unowned prefix is an error, not a silent umu fallback.
func TestUmuRouterOwnerForcedUnownedErrors(t *testing.T) {
	fakeUmuRun(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())

	hook := newUmuLauncher(func(string) string { return "" }, func(string) string { return "owner" }, func() bool { return true }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    filepath.Join(t.TempDir(), "foo.exe"),
	}
	if _, err := hook(context.Background(), row); err == nil {
		t.Error("owner route with unowned prefix: want error, got nil")
	}
}

// TestUmuRouterAutoOwnedMissingCLIErrors: a Bottles-owned prefix with
// no bottles-cli on PATH fails naming the missing CLI (block, never
// silent umu fallback — issue 060).
func TestUmuRouterAutoOwnedMissingCLIErrors(t *testing.T) {
	// PATH must hold ONLY the fake umu-run: the dev box really has
	// bottles-cli installed, and LookPath finding it would defeat the
	// missing-CLI scenario.
	bin := t.TempDir()
	script := "#!/bin/sh\n" +
		`if [ "$1" = "--version" ]; then echo "umu-launcher version 1.4.4 (Python 3.11.6)"; exit 0; fi` + "\n"
	if err := os.WriteFile(filepath.Join(bin, "umu-run"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake umu-run: %v", err)
	}
	t.Setenv("PATH", bin)
	_, bottle := bottlesFixture(t)

	hook := newUmuLauncher(func(string) string { return bottle }, func(string) string { return "" }, func() bool { return true }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    "/games/foo/foo.exe",
	}
	_, err := hook(context.Background(), row)
	if err == nil || !strings.Contains(err.Error(), "bottles-cli") {
		t.Errorf("err = %v, want it naming bottles-cli", err)
	}
}

// TestUmuRouterWritesLaunchLog: a bottles-routed launch writes a
// per-launch log (argv header + child output) under the wired log dir
// (issue 061).
func TestUmuRouterWritesLaunchLog(t *testing.T) {
	fakeUmuRun(t)
	bin := t.TempDir()
	script := "#!/bin/sh\n" +
		`echo "bottles-cli starting"` + "\n"
	if err := os.WriteFile(filepath.Join(bin, "bottles-cli"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake bottles-cli: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, bottle := bottlesFixture(t)

	logDir := t.TempDir()
	hook := newUmuLauncher(func(string) string { return bottle }, func(string) string { return "" }, func() bool { return true }, logDir, func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    "/games/foo/foo.exe",
	}
	if _, err := hook(context.Background(), row); err != nil {
		t.Fatalf("hook: %v", err)
	}

	var content string
	for deadline := time.Now().Add(2 * time.Second); ; {
		matches, _ := filepath.Glob(filepath.Join(logDir, "foo-*.log"))
		if len(matches) == 1 {
			data, _ := os.ReadFile(matches[0])
			content = string(data)
			if strings.Contains(content, "bottles-cli starting") {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("launch log never captured child output; last content:\n%s", content)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(content, "# argv: bottles-cli run -b mybottle -e /games/foo/foo.exe") {
		t.Errorf("log lacks the argv header:\n%s", content)
	}
}

// TestUmuLauncherFailureLogsContext: a failing launch returns umu's
// stderr tail in the error AND logs the full context (game, exe,
// prefix, proton) so the console shows what was attempted.
func TestUmuLauncherFailureLogsContext(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\n" +
		`if [ "$1" = "--version" ]; then echo "umu-launcher version 1.4.4 (Python 3.11.6)"; exit 0; fi` + "\n" +
		`echo "Proton: prefix is broken" >&2; exit 1` + "\n"
	if err := os.WriteFile(filepath.Join(bin, "umu-run"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake umu-run: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	data := t.TempDir()
	t.Setenv("OM_DATA_DIR", data)

	var logBuf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&logBuf)
	t.Cleanup(func() { log.Logger = prev })

	hook := newUmuLauncher(func(string) string { return "/pinned/prefix" }, func(string) string { return "umu" }, func() bool { return true }, "", func(string) string { return "" }, func(context.Context, ui.GameRow) (string, string) { return "umu-default", "" })
	if hook == nil {
		t.Fatal("newUmuLauncher returned nil with umu-run on PATH")
	}
	row := ui.GameRow{
		Title:      "Broken Game",
		InstallDir: t.TempDir(),
		ExePath:    "/games/broken/broken.exe",
	}
	_, err := hook(context.Background(), row)
	if err == nil {
		t.Fatal("hook returned nil for a failing umu-run")
	}
	if !strings.Contains(err.Error(), "Proton: prefix is broken") {
		t.Errorf("error %q lacks umu's stderr tail", err)
	}
	out := logBuf.String()
	for _, want := range []string{"umu launch failed", "Broken Game", "/games/broken/broken.exe", "/pinned/prefix"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output lacks %q:\n%s", want, out)
		}
	}
}
