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
		{"UMU-Proton-10.0-rc1", "10.0"}, // rc stripped, "1" after rc not picked up
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

func TestRunnerRoots_CoversKnownLauncherLayouts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	want := []string{
		filepath.Join(home, ".steam", "steam", "compatibilitytools.d"),
		filepath.Join(home, ".local", "share", "Steam", "compatibilitytools.d"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam", "compatibilitytools.d"),
		filepath.Join(home, "snap", "steam", "common", ".steam", "steam", "compatibilitytools.d"),
		filepath.Join(home, ".local", "share", "bottles", "runners"),
		filepath.Join(home, ".var", "app", "com.usebottles.bottles", "data", "bottles", "runners"),
		filepath.Join(home, ".local", "share", "lutris", "runners", "wine"),
		filepath.Join(home, ".var", "app", "net.lutris.Lutris", "data", "lutris", "runners", "wine"),
		filepath.Join(home, ".config", "heroic", "tools", "proton"),
		filepath.Join(home, ".config", "heroic", "tools", "wine"),
		filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic", "tools", "proton"),
		filepath.Join(home, ".local", "share", "umu", "compatibilitytools"),
	}
	for _, d := range want {
		mkdir(t, d)
	}
	// A plausible-looking dir that is NOT a runner root must not appear.
	mkdir(t, filepath.Join(home, ".local", "share", "lutris", "pga.db"))

	got := RunnerRoots()
	gotSet := map[string]bool{}
	for _, g := range got {
		gotSet[g] = true
	}
	for _, w := range want {
		if !gotSet[w] {
			t.Errorf("RunnerRoots() missing %q (got %v)", w, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("len(RunnerRoots()) = %d, want %d (got %v)", len(got), len(want), got)
	}
}

func TestRunnerRoots_SkipsMissingDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got := RunnerRoots(); len(got) != 0 {
		t.Errorf("RunnerRoots() = %v, want none on an empty HOME", got)
	}
}

func TestRunnerRoots_HonorsXdgOverrides(t *testing.T) {
	home := t.TempDir()
	data := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)

	bottles := filepath.Join(data, "bottles", "runners")
	heroic := filepath.Join(cfg, "heroic", "tools", "proton")
	mkdir(t, bottles)
	mkdir(t, heroic)

	got := RunnerRoots()
	gotSet := map[string]bool{}
	for _, g := range got {
		gotSet[g] = true
	}
	if !gotSet[bottles] {
		t.Errorf("RunnerRoots() missing XDG_DATA_HOME bottles root %q (got %v)", bottles, got)
	}
	if !gotSet[heroic] {
		t.Errorf("RunnerRoots() missing XDG_CONFIG_HOME heroic root %q (got %v)", heroic, got)
	}
}

func TestRunnerRoots_IncludesExtraSteamLibraries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	steam := filepath.Join(home, ".steam", "steam")
	lib := filepath.Join(t.TempDir(), "steamlib")
	writeFile(t, filepath.Join(steam, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+lib+`" } }`)
	compat := filepath.Join(lib, "compatibilitytools.d")
	mkdir(t, compat)

	got := RunnerRoots()
	for _, g := range got {
		if g == compat {
			return
		}
	}
	t.Errorf("RunnerRoots() = %v, want extra-library root %q included", got, compat)
}

func TestRunnerRoots_DeduplicatesSymlinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	real := filepath.Join(home, ".local", "share", "Steam")
	mkdir(t, filepath.Join(real, "compatibilitytools.d"))
	link := filepath.Join(home, ".steam", "steam")
	mkdir(t, filepath.Dir(link))
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	got := RunnerRoots()
	count := 0
	for _, g := range got {
		if filepath.Base(g) == "compatibilitytools.d" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("compatibilitytools.d appears %d times in %v, want 1 (symlink dedup)", count, got)
	}
}

func TestFindRunners_AcceptsProtonScriptWithoutManifest(t *testing.T) {
	root := t.TempDir()

	// GE-Proton repacks for Lutris/Heroic carry no toolmanifest.vdf but
	// ship the top-level proton script umu's PROTONPATH needs.
	ge := filepath.Join(root, "GE-Proton9-25")
	mkdir(t, ge)
	writeFile(t, filepath.Join(ge, "proton"), "#!/bin/sh\n")
	// Plain wine builds (Kron4ek etc.) cannot serve as PROTONPATH.
	wineOnly := filepath.Join(root, "kron4ek-wine-10.0-amd64")
	writeFile(t, filepath.Join(wineOnly, "bin", "wine"), "")

	runners := FindRunners(context.Background(), []string{root})
	if len(runners) != 1 {
		t.Fatalf("len(runners) = %d, want 1 (got %+v)", len(runners), runners)
	}
	if runners[0].Name != "GE-Proton9-25" {
		t.Errorf("runners[0].Name = %q, want GE-Proton9-25", runners[0].Name)
	}
}

func TestResolveProtonPath_PinnedWins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	mkRunner(t, filepath.Join(home, ".local", "share", "Steam", "compatibilitytools.d", "GE-Proton9-3"))

	if got := ResolveProtonPath(context.Background(), "/custom/proton", ""); got != "/custom/proton" {
		t.Errorf("ResolveProtonPath(pinned) = %q, want the pinned path verbatim", got)
	}
}

func TestResolveProtonPath_AutoSelectsNewest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	mkRunner(t, filepath.Join(home, ".local", "share", "Steam", "compatibilitytools.d", "GE-Proton9-3"))
	newest := mkRunner(t, filepath.Join(home, ".config", "heroic", "tools", "proton", "GE-Proton10-1"))

	if got := ResolveProtonPath(context.Background(), "", ""); got != newest {
		t.Errorf("ResolveProtonPath(auto) = %q, want newest detected %q", got, newest)
	}
}

// TestResolveProtonPath_PrefersPrefixSource: a prefix pinned from
// launcher X launches with X's own newest runner, not the global
// newest (issue 059); launchers without runner roots (and Auto) fall
// back to the global newest.
func TestResolveProtonPath_PrefersPrefixSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	steam := mkRunner(t, filepath.Join(home, ".local", "share", "Steam", "compatibilitytools.d", "GE-Proton99-1"))
	bottles := mkRunner(t, filepath.Join(home, ".local", "share", "bottles", "runners", "soda-9.0-1"))

	if got := ResolveProtonPath(context.Background(), "", "bottles"); got != bottles {
		t.Errorf("ResolveProtonPath(prefer bottles) = %q, want the Bottles runner %q", got, bottles)
	}
	if got := ResolveProtonPath(context.Background(), "", "steam"); got != steam {
		t.Errorf("ResolveProtonPath(prefer steam) = %q, want the Steam runner %q", got, steam)
	}
	// Launchers without runner roots fall back to the global newest.
	if got := ResolveProtonPath(context.Background(), "", "playonlinux"); got != steam {
		t.Errorf("ResolveProtonPath(prefer playonlinux) = %q, want global newest %q", got, steam)
	}
	if got := ResolveProtonPath(context.Background(), "", ""); got != steam {
		t.Errorf("ResolveProtonPath(no preference) = %q, want global newest %q", got, steam)
	}
}

func TestResolveProtonPath_EmptyWhenNoRunners(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	if got := ResolveProtonPath(context.Background(), "", "bottles"); got != "" {
		t.Errorf("ResolveProtonPath(auto, no runners) = %q, want empty (umu's own runtime)", got)
	}
}

func TestRootSourceLabelsNewLaunchers(t *testing.T) {
	cases := []struct{ root, want string }{
		{filepath.Join("/home", "u", ".config", "heroic", "tools", "proton"), "heroic"},
		{filepath.Join("/home", "u", ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic", "tools", "wine"), "heroic"},
		{filepath.Join("/home", "u", ".local", "share", "lutris", "runners", "wine"), "lutris"},
		{filepath.Join("/home", "u", ".var", "app", "com.usebottles.bottles", "data", "bottles", "runners"), "bottles"},
		{filepath.Join("/home", "u", ".var", "app", "com.valvesoftware.Steam", "data", "Steam", "compatibilitytools.d"), "steam"},
		{filepath.Join("/games", "steamlib", "compatibilitytools.d"), "steam"},
	}
	for _, c := range cases {
		if got := rootSource(c.root); got != c.want {
			t.Errorf("rootSource(%q) = %q, want %q", c.root, got, c.want)
		}
	}
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
