//go:build linux

package umu

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// mkPrefix creates dir/drive_c (the wine-prefix marker) and returns dir.
func mkPrefix(t *testing.T, dir string) string {
	t.Helper()
	mkdir(t, filepath.Join(dir, "drive_c"))
	return dir
}

func TestFindPrefixes_DetectsLauncherPrefixes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	heroic := mkPrefix(t, filepath.Join(home, "Games", "Heroic", "Prefixes", "SomeApp"))
	bottle := mkPrefix(t, filepath.Join(home, ".local", "share", "bottles", "bottles", "mybottle"))
	steamPfx := mkPrefix(t, filepath.Join(home, ".steam", "steam", "steamapps", "compatdata", "12345", "pfx"))
	wine := mkPrefix(t, filepath.Join(home, ".wine"))
	// Not a prefix: no drive_c inside.
	mkdir(t, filepath.Join(home, ".local", "share", "bottles", "bottles", "empty-bottle"))

	got := FindPrefixes(context.Background())
	byPath := map[string]Prefix{}
	for _, p := range got {
		byPath[p.Path] = p
	}

	checks := []struct {
		path, name, source string
	}{
		{heroic, "SomeApp", "heroic"},
		{bottle, "mybottle", "bottles"},
		{steamPfx, "12345", "steam"},
		{wine, "default", "wine"},
	}
	for _, c := range checks {
		p, ok := byPath[c.path]
		if !ok {
			t.Errorf("FindPrefixes missing %q (got %v)", c.path, got)
			continue
		}
		if p.Name != c.name || p.Source != c.source {
			t.Errorf("prefix %q = {Name:%q Source:%q}, want {%q %q}", c.path, p.Name, p.Source, c.name, c.source)
		}
	}
	if len(got) != len(checks) {
		t.Errorf("len(FindPrefixes) = %d, want %d (got %+v)", len(got), len(checks), got)
	}
}

func TestFindPrefixes_BottlesCustomPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	// Bottles with a user-set bottles directory: data.yml points at the
	// root that holds the prefixes directly (issue 053).
	custom := t.TempDir()
	bottle := mkPrefix(t, filepath.Join(custom, "gaming"))
	dataDir := filepath.Join(home, ".local", "share", "bottles")
	mkdir(t, dataDir)
	writeFile(t, filepath.Join(dataDir, "data.yml"),
		"custom_bottles_path: "+custom+"\nfunding_dialog_dismissed: true\n")

	got := FindPrefixes(context.Background())
	for _, p := range got {
		if p.Path == bottle {
			if p.Source != "bottles" || p.Name != "gaming" {
				t.Errorf("custom bottle = {Name:%q Source:%q}, want {gaming bottles}", p.Name, p.Source)
			}
			return
		}
	}
	t.Errorf("FindPrefixes missing custom Bottles prefix %q (got %v)", bottle, got)
}

func TestFindPrefixes_FlatpakVariants(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	heroic := mkPrefix(t, filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "Games", "Heroic", "Prefixes", "FlatApp"))
	bottle := mkPrefix(t, filepath.Join(home, ".var", "app", "com.usebottles.bottles", "data", "bottles", "bottles", "flatbottle"))

	got := FindPrefixes(context.Background())
	paths := map[string]bool{}
	for _, p := range got {
		paths[p.Path] = true
	}
	for _, want := range []string{heroic, bottle} {
		if !paths[want] {
			t.Errorf("FindPrefixes missing Flatpak prefix %q (got %v)", want, got)
		}
	}
}

func TestFindPrefixes_SteamExtraLibraries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	steam := filepath.Join(home, ".steam", "steam")
	lib := filepath.Join(t.TempDir(), "steamlib")
	writeFile(t, filepath.Join(steam, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+lib+`" } }`)
	pfx := mkPrefix(t, filepath.Join(lib, "steamapps", "compatdata", "999", "pfx"))

	got := FindPrefixes(context.Background())
	for _, p := range got {
		if p.Path == pfx {
			return
		}
	}
	t.Errorf("FindPrefixes = %v, want extra-library compatdata prefix %q", got, pfx)
}

func TestFindPrefixes_EmptyWhenNone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got := FindPrefixes(context.Background()); len(got) != 0 {
		t.Errorf("FindPrefixes = %v, want none on an empty HOME", got)
	}
}

func TestResolvePrefix_PinnedWins(t *testing.T) {
	got, err := ResolvePrefix("/pinned/prefix", "/games/Foo", "/games/Foo/foo.exe", "")
	if err != nil {
		t.Fatalf("ResolvePrefix: %v", err)
	}
	if got != "/pinned/prefix" {
		t.Errorf("ResolvePrefix(pinned) = %q, want the pin verbatim", got)
	}
}

func TestResolvePrefix_EmptyFallsBackToAutoChain(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OM_DATA_DIR", filepath.Join(root, "data"))
	bottle := mkPrefix(t, filepath.Join(root, "bottle"))
	exe := filepath.Join(bottle, "drive_c", "Foo", "foo.exe")

	got, err := ResolvePrefix("", filepath.Join(root, "install"), exe, "")
	if err != nil {
		t.Fatalf("ResolvePrefix: %v", err)
	}
	if got != bottle {
		t.Errorf("ResolvePrefix(auto) = %q, want enclosing prefix %q", got, bottle)
	}
}

func TestStderrTail(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"one", "one"},
		{"a\nb\nc", "a\nb\nc"},
	}
	for _, c := range cases {
		if got := stderrTail(c.in); got != c.want {
			t.Errorf("stderrTail(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// More than 10 lines: only the last 10 non-empty ones survive.
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "line"+string(rune('a'+i)))
	}
	got := stderrTail(strings.Join(lines, "\n"))
	want := strings.Join(lines[10:], "\n")
	if got != want {
		t.Errorf("stderrTail(20 lines) = %q, want last 10 %q", got, want)
	}
	// Blank lines are dropped before tailing.
	if got := stderrTail("first\n\n\nsecond\n\n"); got != "first\nsecond" {
		t.Errorf("stderrTail(blanks) = %q, want %q", got, "first\nsecond")
	}
}

func TestFindPrefixes_SteamNamesFromAppmanifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	steamapps := filepath.Join(home, ".steam", "steam", "steamapps")
	named := mkPrefix(t, filepath.Join(steamapps, "compatdata", "12345", "pfx"))
	writeFile(t, filepath.Join(steamapps, "appmanifest_12345.acf"),
		`"AppState" { "appid" "12345" "name" "Hades" "installdir" "Hades" }`)
	// No manifest for this one: the appid stays the name.
	anon := mkPrefix(t, filepath.Join(steamapps, "compatdata", "99999", "pfx"))

	got := FindPrefixes(context.Background())
	byPath := map[string]Prefix{}
	for _, p := range got {
		byPath[p.Path] = p
	}
	if p := byPath[named]; p.Name != "Hades" || p.Source != "steam" {
		t.Errorf("named steam prefix = %+v, want {Hades steam}", p)
	}
	if p := byPath[anon]; p.Name != "99999" {
		t.Errorf("anonymous steam prefix = %+v, want name 99999", p)
	}
}

func TestFindPrefixes_LutrisYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	pfx := mkPrefix(t, filepath.Join(home, "Games", "some-game"))
	writeFile(t, filepath.Join(home, ".config", "lutris", "games", "some-game-1712345678.yml"),
		"game:\n  exe: "+filepath.Join(pfx, "drive_c", "game.exe")+"\n  prefix: "+pfx+"\nname: Some Game\n")
	// No prefix: key — skipped (Lutris defaults to the install dir,
	// covered by the enclosing-drive_c launch rule).
	writeFile(t, filepath.Join(home, ".config", "lutris", "games", "other-game-1712349999.yml"),
		"game:\n  exe: /games/other/game.exe\nname: Other Game\n")

	got := FindPrefixes(context.Background())
	for _, p := range got {
		if p.Path == pfx && p.Name == "Some Game" && p.Source == "lutris" {
			return
		}
	}
	t.Errorf("FindPrefixes = %+v, want Lutris prefix {Some Game %s lutris}", got, pfx)
}

func TestFindPrefixes_LutrisFlatpakAndXdgConfig(t *testing.T) {
	home := t.TempDir()
	cfg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", cfg)

	xdgPfx := mkPrefix(t, filepath.Join(t.TempDir(), "xdg-game"))
	writeFile(t, filepath.Join(cfg, "lutris", "games", "xdg-game-1.yml"),
		"game:\n  prefix: "+xdgPfx+"\nname: XDG Game\n")
	flatPfx := mkPrefix(t, filepath.Join(t.TempDir(), "flat-game"))
	writeFile(t, filepath.Join(home, ".var", "app", "net.lutris.Lutris", "config", "lutris", "games", "flat-game-2.yml"),
		"game:\n  prefix: "+flatPfx+"\nname: Flat Game\n")

	got := FindPrefixes(context.Background())
	paths := map[string]bool{}
	for _, p := range got {
		paths[p.Path] = true
	}
	for _, want := range []string{xdgPfx, flatPfx} {
		if !paths[want] {
			t.Errorf("FindPrefixes missing Lutris prefix %q (got %+v)", want, got)
		}
	}
}

func TestFindPrefixes_PlayOnLinuxAndCrossOver(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	pol := mkPrefix(t, filepath.Join(home, ".PlayOnLinux", "wineprefix", "office2016"))
	cx := mkPrefix(t, filepath.Join(home, ".cxoffice", "steamgames"))

	got := FindPrefixes(context.Background())
	byPath := map[string]Prefix{}
	for _, p := range got {
		byPath[p.Path] = p
	}
	if p, ok := byPath[pol]; !ok || p.Source != "playonlinux" || p.Name != "office2016" {
		t.Errorf("PlayOnLinux prefix = %+v (present %v), want {office2016 playonlinux}", p, ok)
	}
	if p, ok := byPath[cx]; !ok || p.Source != "crossover" || p.Name != "steamgames" {
		t.Errorf("CrossOver prefix = %+v (present %v), want {steamgames crossover}", p, ok)
	}
}
