package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSized writes a file of exactly size bytes, creating parent dirs.
func writeSized(t *testing.T, path string, size int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	if err := f.Truncate(size); err != nil {
		t.Fatalf("truncate %s: %v", path, err)
	}
}

func TestResolveInstallDirPrefersUE5Win64(t *testing.T) {
	root := t.TempDir()

	ue5 := filepath.Join(root, "Phoenix", "Binaries", "Win64")
	writeSized(t, filepath.Join(ue5, "game.exe"), 100) // tiny exe, would lose scoring
	// A huge, name-similar exe at the root would win scoring — UE5 must still win.
	writeSized(t, filepath.Join(root, filepath.Base(root)+".exe"), 10<<20)

	got, err := ResolveInstallDir(root)
	if err != nil {
		t.Fatalf("ResolveInstallDir: %v", err)
	}
	if got != ue5 {
		t.Fatalf("got %q, want UE5 dir %q", got, ue5)
	}
	t.Logf("UE5 rule won over scored candidates: %s", got)
}

func TestExeScoringSkipsCrashRedistSetup(t *testing.T) {
	root := t.TempDir()

	// The real game exe: large, so +10 size bonus.
	bin := filepath.Join(root, "bin")
	writeSized(t, filepath.Join(bin, "mygame.exe"), 6<<20)
	// All of these must be excluded by name even though some are large.
	writeSized(t, filepath.Join(bin, "mygame_crashhandler.exe"), 6<<20)
	writeSized(t, filepath.Join(root, "tools", "setup.exe"), 6<<20)
	writeSized(t, filepath.Join(root, "redist", "vcredist_x64.exe"), 6<<20)
	writeSized(t, filepath.Join(root, "tools", "installer.exe"), 6<<20)
	writeSized(t, filepath.Join(root, "launcher", "launcher.exe"), 6<<20)
	writeSized(t, filepath.Join(root, "tools", "unins000.exe"), 6<<20)
	// A legitimate but small helper: scores lower than mygame.exe.
	writeSized(t, filepath.Join(root, "tools", "helper.exe"), 1<<20)

	got, err := ResolveInstallDir(root)
	if err != nil {
		t.Fatalf("ResolveInstallDir: %v", err)
	}
	if got != bin {
		t.Fatalf("got %q, want %q", got, bin)
	}
	t.Logf("scoring picked %s, skipping crash/redist/setup/installer/launcher/unins", got)
}

func TestExeScoringBonuses(t *testing.T) {
	t.Run("upscaler adjacency bonus wins", func(t *testing.T) {
		root := t.TempDir()
		// Two equal exes by name/size; dir b also holds a DLSS dll (+25).
		writeSized(t, filepath.Join(root, "a", "tool.exe"), 1<<20)
		writeSized(t, filepath.Join(root, "b", "util.exe"), 1<<20)
		writeFile(t, filepath.Join(root, "b", "nvngx_dlss.dll"), "fake")

		got, err := ResolveInstallDir(root)
		if err != nil {
			t.Fatalf("ResolveInstallDir: %v", err)
		}
		want := filepath.Join(root, "b")
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		t.Logf("upscaler-adjacency bonus won: %s", got)
	})

	t.Run("Binaries/Win64 segment bonus wins", func(t *testing.T) {
		root := t.TempDir()
		writeSized(t, filepath.Join(root, "flat", "tool.exe"), 1<<20)
		win64 := filepath.Join(root, "Engine", "Binaries", "Win64")
		writeSized(t, filepath.Join(win64, "util.exe"), 1<<20)

		got, err := ResolveInstallDir(root)
		if err != nil {
			t.Fatalf("ResolveInstallDir: %v", err)
		}
		if got != win64 {
			t.Fatalf("got %q, want %q", got, win64)
		}
		t.Logf("Binaries/Win64 bonus won: %s", got)
	})

	t.Run("tie breaks to lexicographically smallest path", func(t *testing.T) {
		root := t.TempDir()
		writeSized(t, filepath.Join(root, "zzz", "tool.exe"), 1<<20)
		writeSized(t, filepath.Join(root, "aaa", "tool.exe"), 1<<20)

		got, err := ResolveInstallDir(root)
		if err != nil {
			t.Fatalf("ResolveInstallDir: %v", err)
		}
		want := filepath.Join(root, "aaa")
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		t.Logf("tie broke lexicographically: %s", got)
	})
}

func TestResolveInstallDirZeroCandidates(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveInstallDir(root)
	if err == nil {
		t.Fatal("expected error for game root without exes, got nil")
	}
	if !strings.Contains(err.Error(), root) {
		t.Fatalf("error %q does not name game root %q", err, root)
	}
	t.Logf("zero candidates error: %v", err)
}
