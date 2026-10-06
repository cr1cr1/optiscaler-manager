package archive

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// writeZip builds a zip archive with the given name→content entries at
// path, mirroring what a fork release ships.
func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// A .zip fork bundle extracts through the same pipeline as .7z: entry
// listing, hashing, base-name indexing, and sanitized extraction.
func TestZipExtractsLikeSevenzip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "OptiScaler-NR-v0.8.92.zip")
	writeZip(t, path, map[string]string{
		"OptiScaler.dll":       "fake pe bytes",
		"fakenvapi.dll":        "fake nvapi",
		"amd64/nvngx_dlss.dll": "fake dlss",
	})

	names, err := List(path)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !slices.Contains(names, "OptiScaler.dll") {
		t.Errorf("List = %v, want OptiScaler.dll", names)
	}

	digest, size, err := HashEntry(path, "OptiScaler.dll")
	if err != nil {
		t.Fatalf("HashEntry: %v", err)
	}
	if size != int64(len("fake pe bytes")) || len(digest) != 64 {
		t.Errorf("HashEntry size=%d digest=%q, want %d bytes hashed", size, digest, len("fake pe bytes"))
	}

	base, err := EntryNames(path)
	if err != nil {
		t.Fatalf("EntryNames: %v", err)
	}
	for _, want := range []string{"optiscaler.dll", "fakenvapi.dll", "nvngx_dlss.dll"} {
		if !slices.Contains(base, want) {
			t.Errorf("EntryNames = %v, want %q", base, want)
		}
	}

	dst := t.TempDir()
	if err := ExtractTo(path, dst); err != nil {
		t.Fatalf("ExtractTo: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "OptiScaler.dll"))
	if err != nil {
		t.Fatalf("extracted OptiScaler.dll missing: %v", err)
	}
	if string(got) != "fake pe bytes" {
		t.Errorf("extracted bytes %q, want the zip payload", got)
	}
	if _, err := os.Stat(filepath.Join(dst, "amd64", "nvngx_dlss.dll")); err != nil {
		t.Errorf("nested entry missing after extraction: %v", err)
	}
	t.Logf("zip list/hash/extract ok (%d entries)", len(names))
}

// The hostile-input defenses apply to zip too: a traversal entry aborts
// the extraction and writes nothing outside the destination.
func TestZipTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evil.zip")
	writeZip(t, path, map[string]string{"../evil.dll": "boom"})

	dst := t.TempDir()
	if err := ExtractTo(path, dst); err == nil {
		t.Fatal("ExtractTo with a traversal entry expected error, got nil")
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.dll")); err == nil {
		t.Error("traversal entry escaped the destination directory")
	}
}

// An unsupported extension fails loud instead of being sniffed as 7z.
func TestUnknownExtensionRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.rar")
	if err := os.WriteFile(path, []byte("not an archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := List(path); err == nil {
		t.Error("List(.rar) succeeded, want an unsupported-format error")
	}
}
