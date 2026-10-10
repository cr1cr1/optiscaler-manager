package launchlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateMakesSluggedTimestampedFile: the log file lands in dir
// (created on demand), named after the slugged game so the user can
// find it, unique per launch.
func TestCreateMakesSluggedTimestampedFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs") // does not exist yet
	path, f, err := Create(dir, "Resonance: Deluxe!")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	base := filepath.Base(path)
	if filepath.Dir(path) != dir {
		t.Errorf("path %q not in dir %q", path, dir)
	}
	if !strings.HasPrefix(base, "resonance-deluxe-") || !strings.HasSuffix(base, ".log") {
		t.Errorf("file %q lacks slug prefix / .log suffix", base)
	}

	path2, f2, err := Create(dir, "Resonance: Deluxe!")
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	_ = f2.Close()
	if path2 == path {
		t.Errorf("two launches within the test got the same file %q", path)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Resonance":       "resonance",
		"Some Game: 2":    "some-game-2",
		"  Weird@@Name  ": "weird-name",
		"":                "game",
		"!!!":             "game",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestWriteHeader: the header records the exact argv and the env delta
// the launcher set, so "what did we actually tell the child" is never
// a guessing game.
func TestWriteHeader(t *testing.T) {
	var b strings.Builder
	WriteHeader(&b, []string{"bottles-cli", "run", "-b", "mybottle", "-e", "/games/foo.exe"},
		[]string{"GAMEID=umu-default", "WINEPREFIX=/pfx"})
	out := b.String()
	for _, want := range []string{
		"# argv: bottles-cli run -b mybottle -e /games/foo.exe",
		"GAMEID=umu-default",
		"WINEPREFIX=/pfx",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("header lacks %q:\n%s", want, out)
		}
	}
}

// TestCreateEmptyDirFailsCleanly: an empty dir means "no logging" is
// the caller's choice, not ours — Create still must not panic.
func TestCreateEmptyDir(t *testing.T) {
	_, f, err := Create("", "x")
	if err == nil {
		_ = f.Close()
		t.Error("Create with empty dir: want error, got nil")
	}
	if _, err := os.Stat(""); err == nil {
		t.Error("unexpected")
	}
}
