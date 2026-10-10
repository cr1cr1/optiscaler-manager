//go:build linux

package launchercli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeCLI installs a fake binary on PATH that records its argv (one
// argument per line) into the file named by CLI_TEST_CAPTURE.
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

func readArgv(t *testing.T, capture string) []string {
	t.Helper()
	// The spawn is detached (Start + Release): poll until the child has
	// written its argv.
	var data []byte
	for deadline := time.Now().Add(2 * time.Second); ; {
		var err error
		data, err = os.ReadFile(capture)
		if err == nil && len(data) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("capture %s never written (last err: %v)", capture, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// TestLaunchArgv: each owner backend invokes its launcher CLI with the
// documented argv, native and Flatpak forms (issue 060).
func TestLaunchArgv(t *testing.T) {
	cases := []struct {
		name        string
		source, ref string
		exe         string
		flatpak     bool
		fakeBin     string
		wantArgv    []string
	}{
		{"bottles native", "bottles", "mybottle", "/games/foo/foo.exe", false, "bottles-cli",
			[]string{"run", "-b", "mybottle", "-e", "/games/foo/foo.exe"}},
		{"bottles flatpak", "bottles", "mybottle", "/games/foo/foo.exe", true, "flatpak",
			[]string{"run", "--command=bottles-cli", "com.usebottles.bottles", "run", "-b", "mybottle", "-e", "/games/foo/foo.exe"}},
		{"lutris native", "lutris", "my-game", "", false, "lutris",
			[]string{"lutris:rungame/my-game"}},
		{"lutris flatpak", "lutris", "my-game", "", true, "flatpak",
			[]string{"run", "net.lutris.Lutris", "lutris:rungame/my-game"}},
		{"heroic native", "heroic", "Fortnite", "", false, "heroic",
			[]string{"launch", "Fortnite"}},
		{"heroic flatpak", "heroic", "Fortnite", "", true, "flatpak",
			[]string{"run", "com.heroicgameslauncher.hgl", "launch", "Fortnite"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			capture := fakeCLI(t, c.fakeBin)
			if err := Launch(context.Background(), c.source, c.ref, c.exe, c.flatpak); err != nil {
				t.Fatalf("Launch: %v", err)
			}
			got := readArgv(t, capture)
			if strings.Join(got, " ") != strings.Join(c.wantArgv, " ") {
				t.Errorf("argv = %v, want %v", got, c.wantArgv)
			}
		})
	}
}

// TestLaunchErrors: unroutable sources/empty refs and missing CLIs
// fail with errors naming owner and remedy — never a silent spawn.
func TestLaunchErrors(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no CLIs at all

	if err := Launch(context.Background(), "bottles", "mybottle", "/games/foo.exe", false); err == nil ||
		!strings.Contains(err.Error(), "bottles-cli") {
		t.Errorf("missing bottles-cli error = %v, want it naming bottles-cli", err)
	}
	if err := Launch(context.Background(), "steam", "", "", false); err == nil ||
		!strings.Contains(err.Error(), "Steam") {
		t.Errorf("steam error = %v, want it naming Steam", err)
	}
	if err := Launch(context.Background(), "bottles", "", "/games/foo.exe", false); err == nil {
		t.Error("empty bottle ref: want error, got nil")
	}
}

// TestCommandPure: the argv builder is pure and identical off-PATH.
func TestCommandPure(t *testing.T) {
	if _, err := Command("playonlinux", "x", "", false); err == nil {
		t.Error("playonlinux: want unroutable error, got nil")
	}
	argv, err := Command("bottles", "b", "/x.exe", false)
	if err != nil || strings.Join(argv, " ") != "bottles-cli run -b b -e /x.exe" {
		t.Errorf("Command(bottles) = %v, %v", argv, err)
	}
}
