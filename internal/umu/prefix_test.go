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

// shaSlug is the test-only reference implementation of the slug,
// re-declared here so a future edit of Slug's hash params breaks the
// test instead of silently changing the directory layout on disk.
func shaSlug(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])[:12]
}

var _ = os.Setenv
