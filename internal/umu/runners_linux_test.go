//go:build linux

package umu

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFindRunners_ScansSteamBottlesAndUmu(t *testing.T) {
	root := t.TempDir()

	// Three valid runners, one in each known location. Each has a
	// toolmanifest.vdf so FindRunners treats it as installable.
	// Path names use the real-world capitalization: Steam (capital S),
	// bottles (lowercase), umu (lowercase).
	mkRunner(t, filepath.Join(root, "Steam", "compatibilitytools.d", "GE-Proton9-3"))
	mkRunner(t, filepath.Join(root, "bottles", "runners", "ge-proton9-2"))
	mkRunner(t, filepath.Join(root, "umu", "compatibilitytools", "UMU-Proton-10.0-1"))

	runners := FindRunners(context.Background(), []string{
		filepath.Join(root, "Steam", "compatibilitytools.d"),
		filepath.Join(root, "bottles", "runners"),
		filepath.Join(root, "umu", "compatibilitytools"),
	})

	if got, want := len(runners), 3; got != want {
		t.Fatalf("len(runners) = %d, want %d (got %+v)", got, want, runners)
	}

	// Versions sort descending: 10.0.1 > 9.3 > 9.2
	if got, want := runners[0].Version, "10.0.1"; got != want {
		t.Errorf("runners[0].Version = %q, want %q", got, want)
	}
	if runners[0].Source != "umu" {
		t.Errorf("runners[0].Source = %q, want umu", runners[0].Source)
	}
	if runners[1].Version != "9.3" {
		t.Errorf("runners[1].Version = %q, want 9.3", runners[1].Version)
	}
	if runners[1].Source != "steam" {
		t.Errorf("runners[1].Source = %q, want steam", runners[1].Source)
	}
	if runners[2].Version != "9.2" {
		t.Errorf("runners[2].Version = %q, want 9.2", runners[2].Version)
	}
	if runners[2].Source != "bottles" {
		t.Errorf("runners[2].Source = %q, want bottles", runners[2].Source)
	}

	// Path is the runner directory itself.
	if want := filepath.Join(root, "Steam", "compatibilitytools.d", "GE-Proton9-3"); runners[1].Path != want {
		t.Errorf("runners[1].Path = %q, want %q", runners[1].Path, want)
	}
	// Name is the directory basename.
	if runners[1].Name != "GE-Proton9-3" {
		t.Errorf("runners[1].Name = %q, want GE-Proton9-3", runners[1].Name)
	}
}

func TestFindRunners_SkipsInvalidDirs(t *testing.T) {
	root := t.TempDir()
	steamDir := filepath.Join(root, "compatibilitytools.d")

	// Valid: has toolmanifest.vdf.
	mkRunner(t, filepath.Join(steamDir, "GE-Proton9-3"))
	// Invalid: no toolmanifest.vdf.
	badDir := filepath.Join(steamDir, "broken-runner")
	mkdir(t, badDir)
	// Invalid: random file with no manifest.
	writeFile(t, filepath.Join(steamDir, "stray-file.txt"), "ignore me")

	runners := FindRunners(context.Background(), []string{steamDir})
	if got := len(runners); got != 1 {
		t.Fatalf("len(runners) = %d, want 1 (invalid dirs must be skipped)", got)
	}
	if runners[0].Name != "GE-Proton9-3" {
		t.Errorf("runners[0].Name = %q, want GE-Proton9-3", runners[0].Name)
	}
}

func TestFindRunners_EmptyWhenNoDirs(t *testing.T) {
	runners := FindRunners(context.Background(), nil)
	if len(runners) != 0 {
		t.Fatalf("len(runners) = %d, want 0 for nil roots", len(runners))
	}

	// Nonexistent roots: no error, no results.
	runners = FindRunners(context.Background(), []string{"/this/does/not/exist"})
	if len(runners) != 0 {
		t.Fatalf("len(runners) = %d, want 0 for missing root", len(runners))
	}
}

func TestFindRunners_DeduplicatesAcrossRoots(t *testing.T) {
	// The same runner name appearing under two roots must be returned
	// twice (different paths), since the user may have installed the
	// same version in multiple locations. Sort still applies.
	root := t.TempDir()
	r1 := mkRunner(t, filepath.Join(root, "a", "GE-Proton9-3"))
	r2 := mkRunner(t, filepath.Join(root, "b", "GE-Proton9-3"))

	runners := FindRunners(context.Background(), []string{
		filepath.Join(root, "a"),
		filepath.Join(root, "b"),
	})
	if len(runners) != 2 {
		t.Fatalf("len(runners) = %d, want 2", len(runners))
	}
	// Both entries should have the same version, so the sort key is
	// stable; just confirm both paths are present.
	paths := map[string]bool{runners[0].Path: true, runners[1].Path: true}
	if !paths[r1] || !paths[r2] {
		t.Errorf("runners paths = %v, want both %q and %q", paths, r1, r2)
	}
}

func TestParseRunnerVersion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"GE-Proton9-3", "9.3"},
		{"GE-Proton9-20", "9.20"},
		{"UMU-Proton-10.0-1", "10.0.1"},
		{"UMU-Proton-10.0-rc1", "10.0"},  // rc stripped, "1" after rc not picked up
		{"proton-9.0-beta", "9.0"},      // beta stripped, no trailing num
		{"soda-9.0-2", "9.0.2"},
		{"wine-ge-8.26", "8.26"},
		{"random-name", "0"},
		{"", "0"},
	}
	for _, c := range cases {
		got := parseRunnerVersion(c.in)
		if got != c.want {
			t.Errorf("parseRunnerVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// mkRunner creates a directory containing the magic toolmanifest.vdf
// file that signals "this is a usable Proton runner", and returns the
// directory path.
func mkRunner(t *testing.T, dir string) string {
	t.Helper()
	mkdir(t, dir)
	writeFile(t, filepath.Join(dir, "toolmanifest.vdf"),
		`"manifest" { "version" "0" }`)
	return dir
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
