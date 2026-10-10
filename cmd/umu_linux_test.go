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
// WINEPREFIX environment into the file named by UMU_TEST_CAPTURE.
func fakeUmuRun(t *testing.T) (capture string) {
	t.Helper()
	bin := t.TempDir()
	capture = filepath.Join(t.TempDir(), "capture")
	script := "#!/bin/sh\n" +
		`if [ "$1" = "--version" ]; then echo "umu-launcher version 1.4.4 (Python 3.11.6)"; exit 0; fi` + "\n" +
		`printf '%s\n%s\n' "$PROTONPATH" "$WINEPREFIX" > "$UMU_TEST_CAPTURE"` + "\n"
	if err := os.WriteFile(filepath.Join(bin, "umu-run"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake umu-run: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("UMU_TEST_CAPTURE", capture)
	return capture
}

func readCapture(t *testing.T, capture string) (proton, prefix string) {
	t.Helper()
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("capture = %q, want two lines (PROTONPATH, WINEPREFIX)", string(data))
	}
	return lines[0], lines[1]
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

	hook := newUmuLauncher(func(string) string { return "" }, func(string) string { return "" }, func() bool { return true }, "")
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

	hook := newUmuLauncher(func(string) string { return "" }, func(string) string { return "" }, func() bool { return true }, "")
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

// TestUmuLauncherPerGamePinWins: the live pin resolver's result becomes
// WINEPREFIX verbatim (per-game pin > global default > auto — the
// precedence is the resolver's job, tested in internal/ui).
func TestUmuLauncherPerGamePinWins(t *testing.T) {
	capture := fakeUmuRun(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())

	hook := newUmuLauncher(func(string) string { return "/pinned/prefix" }, func(string) string { return "umu" }, func() bool { return true }, "")
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

// TestUmuLauncherRunnerFollowsPrefixSource: a pinned Bottles prefix
// launches with the newest runner from BOTTLES' runner root, not the
// global newest (issue 059).
func TestUmuLauncherRunnerFollowsPrefixSource(t *testing.T) {
	capture := fakeUmuRun(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())

	// A much newer Steam runner that must NOT win for a Bottles prefix.
	steamRunner := filepath.Join(home, ".local", "share", "Steam", "compatibilitytools.d", "GE-Proton99-1")
	bottlesRunner := filepath.Join(home, ".local", "share", "bottles", "runners", "soda-9.0-1")
	for _, r := range []string{steamRunner, bottlesRunner} {
		if err := os.MkdirAll(r, 0o755); err != nil {
			t.Fatalf("mkdir runner: %v", err)
		}
		if err := os.WriteFile(filepath.Join(r, "proton"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write proton script: %v", err)
		}
	}
	bottle := filepath.Join(home, ".local", "share", "bottles", "bottles", "mybottle")
	if err := os.MkdirAll(filepath.Join(bottle, "drive_c"), 0o755); err != nil {
		t.Fatalf("mkdir bottle: %v", err)
	}

	// Force the umu route: under auto a Bottles-owned prefix launches
	// via bottles-cli instead (issue 060).
	hook := newUmuLauncher(func(string) string { return bottle }, func(string) string { return "umu" }, func() bool { return true }, "")
	if hook == nil {
		t.Fatal("newUmuLauncher returned nil")
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
	if proton != bottlesRunner {
		t.Errorf("PROTONPATH = %q, want the Bottles runner %q (not global newest %q)", proton, bottlesRunner, steamRunner)
	}
	if prefix != bottle {
		t.Errorf("WINEPREFIX = %q, want the pinned bottle %q", prefix, bottle)
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

	hook := newUmuLauncher(func(string) string { return bottle }, func(string) string { return "" }, func() bool { return true }, "")
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

	hook := newUmuLauncher(func(string) string { return "" }, func(string) string { return "" }, func() bool { return true }, "")
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

	hook := newUmuLauncher(func(string) string { return "" }, func(string) string { return "" }, func() bool { return false }, "")
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

	hook := newUmuLauncher(func(string) string { return "" }, func(string) string { return "owner" }, func() bool { return true }, "")
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

	hook := newUmuLauncher(func(string) string { return bottle }, func(string) string { return "" }, func() bool { return true }, "")
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
	hook := newUmuLauncher(func(string) string { return bottle }, func(string) string { return "" }, func() bool { return true }, logDir)
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

	hook := newUmuLauncher(func(string) string { return "/pinned/prefix" }, func(string) string { return "umu" }, func() bool { return true }, "")
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
