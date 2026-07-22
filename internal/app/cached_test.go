package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeFile is a tiny helper so the table entries stay declarative.
func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestCachedVersions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, cacheDir string) (dir string)
		want  []string
	}{
		{
			name: "missing cache dir returns nil",
			setup: func(t *testing.T, cacheDir string) string {
				return filepath.Join(cacheDir, "does-not-exist")
			},
			want: nil,
		},
		{
			name: "empty optiscaler dir returns nil",
			setup: func(t *testing.T, cacheDir string) string {
				if err := os.MkdirAll(filepath.Join(cacheDir, "optiscaler"), 0o755); err != nil {
					t.Fatal(err)
				}
				return cacheDir
			},
			want: nil,
		},
		{
			name: "version dir with valid bundle is included verbatim",
			setup: func(t *testing.T, cacheDir string) string {
				writeFile(t, filepath.Join(cacheDir, "optiscaler", "v0.9.4",
					"Optiscaler_0.9.4-final.20260718._MM.7z"))
				return cacheDir
			},
			want: []string{"v0.9.4"},
		},
		{
			name: "version dir with only non-matching files is excluded",
			setup: func(t *testing.T, cacheDir string) string {
				writeFile(t, filepath.Join(cacheDir, "optiscaler", "v0.9.4", ".download-123"))
				writeFile(t, filepath.Join(cacheDir, "optiscaler", "v0.9.4", "notes.txt"))
				return cacheDir
			},
			want: nil,
		},
		{
			name: "regular file named like a version is excluded",
			setup: func(t *testing.T, cacheDir string) string {
				writeFile(t, filepath.Join(cacheDir, "optiscaler", "v0.9.4"))
				return cacheDir
			},
			want: nil,
		},
		{
			name: "multiple versions sorted newest first (numeric, not lexicographic)",
			setup: func(t *testing.T, cacheDir string) string {
				writeFile(t, filepath.Join(cacheDir, "optiscaler", "v0.9.4", "Optiscaler_a.7z"))
				writeFile(t, filepath.Join(cacheDir, "optiscaler", "v0.10.0", "Optiscaler_b.7z"))
				writeFile(t, filepath.Join(cacheDir, "optiscaler", "v0.9.10", "Optiscaler_c.7z"))
				return cacheDir
			},
			want: []string{"v0.10.0", "v0.9.10", "v0.9.4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheDir := t.TempDir()
			dir := tt.setup(t, cacheDir)
			got := CachedVersions(dir)
			t.Logf("CachedVersions(%q) = %v", dir, got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CachedVersions(%q) = %v, want %v", dir, got, tt.want)
			}
		})
	}
}
