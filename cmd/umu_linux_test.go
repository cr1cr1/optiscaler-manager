//go:build linux

package optiscalermanager

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	hook := newUmuLauncher(func(string) string { return "" })
	if hook == nil {
		t.Fatal("newUmuLauncher returned nil with umu-run on PATH")
	}
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: filepath.Join(t.TempDir(), "Foo"),
		ExePath:    filepath.Join(t.TempDir(), "Foo", "foo.exe"),
	}
	if err := hook(context.Background(), row); err != nil {
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

	hook := newUmuLauncher(func(string) string { return "" })
	if hook == nil {
		t.Fatal("newUmuLauncher returned nil with umu-run on PATH")
	}
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: install,
		ExePath:    filepath.Join(install, "foo.exe"),
	}
	if err := hook(context.Background(), row); err != nil {
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

	hook := newUmuLauncher(func(string) string { return "/pinned/prefix" })
	if hook == nil {
		t.Fatal("newUmuLauncher returned nil with umu-run on PATH")
	}
	row := ui.GameRow{
		Title:      "Foo",
		InstallDir: t.TempDir(),
		ExePath:    filepath.Join(t.TempDir(), "foo.exe"),
	}
	if err := hook(context.Background(), row); err != nil {
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

	hook := newUmuLauncher(func(string) string { return "/pinned/prefix" })
	if hook == nil {
		t.Fatal("newUmuLauncher returned nil with umu-run on PATH")
	}
	row := ui.GameRow{
		Title:      "Broken Game",
		InstallDir: t.TempDir(),
		ExePath:    "/games/broken/broken.exe",
	}
	err := hook(context.Background(), row)
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
