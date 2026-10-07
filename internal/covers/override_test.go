package covers

import (
	"os"
	"path/filepath"
	"testing"
)

// Issue 028: a user-picked poster is copied into the cache under a stable
// per-dir override name and normalized to the 2:3 invariant like any
// fetched art.
func TestSetOverrideCopiesAndNormalizes(t *testing.T) {
	c := New(nil, t.TempDir())
	src := filepath.Join(t.TempDir(), "art.jpg")
	if err := os.WriteFile(src, wideJPEG(t), 0o644); err != nil { // 300x90 landscape
		t.Fatal(err)
	}

	name, err := c.SetOverride("/games/Foo", src)
	if err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	p, ok := c.OverridePath(name)
	if !ok {
		t.Fatalf("OverridePath(%q) not found after SetOverride", name)
	}
	img := decodeImg(t, p)
	b := img.Bounds()
	if b.Dx()*3 != b.Dy()*2 {
		t.Errorf("override cached at %dx%d, want 2:3 portrait", b.Dx(), b.Dy())
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source file must be left untouched (copy, not move): %v", err)
	}
}

// Issue 028: re-uploading replaces the same cache file (stable name), so
// the settings map never churns.
func TestSetOverrideStableNameReplaces(t *testing.T) {
	c := New(nil, t.TempDir())
	dir := t.TempDir()
	src1 := filepath.Join(dir, "a.jpg")
	src2 := filepath.Join(dir, "b.jpg")
	if err := os.WriteFile(src1, wideJPEG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src2, wideJPEG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	n1, err := c.SetOverride("/games/Foo", src1)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := c.SetOverride("/games/Foo", src2)
	if err != nil {
		t.Fatal(err)
	}
	if n1 != n2 {
		t.Errorf("override name unstable: %q then %q", n1, n2)
	}
	entries, err := filepath.Glob(filepath.Join(c.cacheDir, "override_*.img"))
	if err != nil || len(entries) != 1 {
		t.Errorf("override files = %v (err %v), want exactly 1", entries, err)
	}
}

// Issue 028: undecodable bytes are rejected — no cache file, no override.
func TestSetOverrideRejectsUndecodable(t *testing.T) {
	c := New(nil, t.TempDir())
	src := filepath.Join(t.TempDir(), "junk.img")
	if err := os.WriteFile(src, []byte("not an image at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetOverride("/games/Foo", src); err == nil {
		t.Fatal("SetOverride(junk) = nil error, want rejection")
	}
	entries, _ := filepath.Glob(filepath.Join(c.cacheDir, "override_*.img"))
	if len(entries) != 0 {
		t.Errorf("rejected upload left cache files: %v", entries)
	}
}

// Issue 028: clearing an override removes the cached copy so the fetch
// chain takes over again.
func TestClearOverrideRemovesCacheFile(t *testing.T) {
	c := New(nil, t.TempDir())
	src := filepath.Join(t.TempDir(), "art.jpg")
	if err := os.WriteFile(src, wideJPEG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	name, err := c.SetOverride("/games/Foo", src)
	if err != nil {
		t.Fatal(err)
	}
	c.ClearOverride(name)
	if _, ok := c.OverridePath(name); ok {
		t.Errorf("OverridePath(%q) still present after ClearOverride", name)
	}
}
