//go:build linux

package optiscalermanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
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

	hook := newUmuLauncher(settings.Settings{})
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

	hook := newUmuLauncher(settings.Settings{})
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

// TestUmuLauncherPinnedProtonWins: a non-empty umu_proton_path setting
// is passed through verbatim, ignoring detected runners.
func TestUmuLauncherPinnedProtonWins(t *testing.T) {
	capture := fakeUmuRun(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OM_DATA_DIR", t.TempDir())

	hook := newUmuLauncher(settings.Settings{UmuProtonPath: "/custom/proton"})
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

	proton, _ := readCapture(t, capture)
	if proton != "/custom/proton" {
		t.Errorf("PROTONPATH = %q, want pinned /custom/proton", proton)
	}
}
