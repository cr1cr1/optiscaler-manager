package umu

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrefixFor_IsDeterministic(t *testing.T) {
	a, err := PrefixFor("/games/Foo", "Foo")
	if err != nil {
		t.Fatalf("PrefixFor: %v", err)
	}
	b, err := PrefixFor("/games/Foo", "Foo")
	if err != nil {
		t.Fatalf("PrefixFor second call: %v", err)
	}
	if a != b {
		t.Errorf("nondeterministic: got %q then %q", a, b)
	}
}

func TestPrefixFor_DiffersByInstallDir(t *testing.T) {
	a, err := PrefixFor("/games/Foo", "Foo")
	if err != nil {
		t.Fatalf("PrefixFor A: %v", err)
	}
	b, err := PrefixFor("/games/Bar", "Bar")
	if err != nil {
		t.Fatalf("PrefixFor B: %v", err)
	}
	if a == b {
		t.Errorf("collisions: both games mapped to %q", a)
	}
}

func TestPrefixFor_IgnoresGameNameSameDirSamePrefix(t *testing.T) {
	// Two games installed in the same dir (rare but possible) get the
	// same prefix; installDir is the identity key.
	a, _ := PrefixFor("/games/Foo", "Foo")
	b, _ := PrefixFor("/games/Foo", "Different Name")
	if a != b {
		t.Errorf("same installDir produced different prefixes: %q vs %q", a, b)
	}
}

func TestPrefixFor_HandlesSpacesAndUnicode(t *testing.T) {
	p, err := PrefixFor("/games/My Gåme", "My Gåme")
	if err != nil {
		t.Fatalf("PrefixFor: %v", err)
	}
	if strings.ContainsAny(p, " ") {
		t.Errorf("prefix contains a space: %q", p)
	}
	for _, r := range p {
		if r > 127 {
			t.Errorf("prefix contains non-ASCII rune %q in %q", r, p)
		}
	}
}

func TestPrefixFor_HonorsOMDataDirOverride(t *testing.T) {
	// OM_DATA_DIR is the FULL root (per cmd/deps.go + store.DefaultRoot):
	// it does NOT get /optiscaler-manager appended.
	t.Setenv("OM_DATA_DIR", "/custom/data")
	p, err := PrefixFor("/games/Foo", "Foo")
	if err != nil {
		t.Fatalf("PrefixFor: %v", err)
	}
	if want := filepath.Join("/custom/data", "umu-prefixes", shaSlug("/games/Foo")); p != want {
		t.Errorf("prefix = %q, want %q", p, want)
	}
}

func TestPrefixFor_DirectoryPathMatchesSlug(t *testing.T) {
	dir := "/games/Some Game With Spaces"
	p, err := PrefixFor(dir, "name")
	if err != nil {
		t.Fatalf("PrefixFor: %v", err)
	}
	slug := shaSlug(dir)
	if !strings.HasSuffix(p, slug) {
		t.Errorf("prefix %q doesn't end with slug %q", p, slug)
	}
}

func TestPrefixFor_UsesXdgDataHomeWhenNoOverride(t *testing.T) {
	// OM_DATA_DIR unset; falls back to XDG_DATA_HOME/optiscaler-manager.
	t.Setenv("OM_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/home/me/.local/share")
	p, err := PrefixFor("/games/Foo", "Foo")
	if err != nil {
		t.Fatalf("PrefixFor: %v", err)
	}
	if want := filepath.Join("/home/me/.local/share", "optiscaler-manager", "umu-prefixes", shaSlug("/games/Foo")); p != want {
		t.Errorf("prefix = %q, want %q", p, want)
	}
}

func TestPrefixFor_FallsBackToHomeShare(t *testing.T) {
	t.Setenv("OM_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/home/user")
	p, err := PrefixFor("/games/Foo", "Foo")
	if err != nil {
		t.Fatalf("PrefixFor: %v", err)
	}
	if want := filepath.Join("/home/user/.local/share", "optiscaler-manager", "umu-prefixes", shaSlug("/games/Foo")); p != want {
		t.Errorf("prefix = %q, want %q", p, want)
	}
}

func TestPrefixForGame_ReusesEnclosingPrefix(t *testing.T) {
	root := t.TempDir()
	prefix := filepath.Join(root, "bottles", "mybottle")
	mustMkdir(t, filepath.Join(prefix, "drive_c", "Games", "Foo"))
	exe := filepath.Join(prefix, "drive_c", "Games", "Foo", "foo.exe")

	got, err := PrefixForGame(filepath.Join(root, "elsewhere"), exe, "")
	if err != nil {
		t.Fatalf("PrefixForGame: %v", err)
	}
	if got != prefix {
		t.Errorf("prefix = %q, want enclosing prefix %q", got, prefix)
	}
}

func TestPrefixForGame_EnclosingPrefixMustExist(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OM_DATA_DIR", filepath.Join(root, "data"))
	// drive_c in the path but the prefix root is NOT on disk.
	exe := filepath.Join(root, "ghost", "drive_c", "Foo", "foo.exe")

	got, err := PrefixForGame(filepath.Join(root, "install"), exe, "")
	if err != nil {
		t.Fatalf("PrefixForGame: %v", err)
	}
	want, _ := PrefixFor(filepath.Join(root, "install"), "")
	if got != want {
		t.Errorf("prefix = %q, want app-managed %q when the enclosing prefix is missing", got, want)
	}
}

func TestPrefixForGame_ReusesHeroicPrefix(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("HOME", home)
	t.Setenv("OM_DATA_DIR", filepath.Join(root, "data"))
	heroic := filepath.Join(home, "Games", "Heroic", "Prefixes", "SomeApp")
	mustMkdir(t, heroic)

	got, err := PrefixForGame(filepath.Join(root, "install"), filepath.Join(root, "install", "game.exe"), "SomeApp")
	if err != nil {
		t.Fatalf("PrefixForGame: %v", err)
	}
	if got != heroic {
		t.Errorf("prefix = %q, want existing Heroic prefix %q", got, heroic)
	}
}

func TestPrefixForGame_ReusesHeroicFlatpakPrefix(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("HOME", home)
	t.Setenv("OM_DATA_DIR", filepath.Join(root, "data"))
	heroic := filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "Games", "Heroic", "Prefixes", "SomeApp")
	mustMkdir(t, heroic)

	got, err := PrefixForGame(filepath.Join(root, "install"), filepath.Join(root, "install", "game.exe"), "SomeApp")
	if err != nil {
		t.Fatalf("PrefixForGame: %v", err)
	}
	if got != heroic {
		t.Errorf("prefix = %q, want existing Heroic Flatpak prefix %q", got, heroic)
	}
}

func TestPrefixForGame_HeroicPrefixMustExist(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("OM_DATA_DIR", filepath.Join(root, "data"))

	got, err := PrefixForGame(filepath.Join(root, "install"), filepath.Join(root, "install", "game.exe"), "SomeApp")
	if err != nil {
		t.Fatalf("PrefixForGame: %v", err)
	}
	want, _ := PrefixFor(filepath.Join(root, "install"), "")
	if got != want {
		t.Errorf("prefix = %q, want app-managed %q when no Heroic prefix exists", got, want)
	}
}

func TestPrefixForGame_EnclosingBeatsHeroic(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("HOME", home)
	t.Setenv("OM_DATA_DIR", filepath.Join(root, "data"))
	prefix := filepath.Join(root, "bottle")
	mustMkdir(t, filepath.Join(prefix, "drive_c", "Foo"))
	mustMkdir(t, filepath.Join(home, "Games", "Heroic", "Prefixes", "SomeApp"))

	got, err := PrefixForGame(filepath.Join(root, "install"), filepath.Join(prefix, "drive_c", "Foo", "foo.exe"), "SomeApp")
	if err != nil {
		t.Fatalf("PrefixForGame: %v", err)
	}
	if got != prefix {
		t.Errorf("prefix = %q, want enclosing prefix %q to win over Heroic", got, prefix)
	}
}

// mustMkdir creates dir (and parents) or fails the test.
func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

// shaSlug is the test-only reference implementation of the slug,
// re-declared here so a future edit of Slug's hash params breaks the
// test instead of silently changing the directory layout on disk.
func shaSlug(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])[:12]
}

var _ = os.Setenv
