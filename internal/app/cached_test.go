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
		name    string
		pattern string
		setup   func(t *testing.T, bundleDir string)
		want    []string
	}{
		{
			name:    "missing bundle dir returns nil",
			pattern: "Optiscaler_*.7z",
			setup:   func(t *testing.T, bundleDir string) {},
			want:    nil,
		},
		{
			name:    "empty bundle dir returns nil",
			pattern: "Optiscaler_*.7z",
			setup: func(t *testing.T, bundleDir string) {
				if err := os.MkdirAll(bundleDir, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: nil,
		},
		{
			name:    "version dir with valid bundle is included verbatim",
			pattern: "Optiscaler_*.7z",
			setup: func(t *testing.T, bundleDir string) {
				writeFile(t, filepath.Join(bundleDir, "v0.9.4",
					"Optiscaler_0.9.4-final.20260718._MM.7z"))
			},
			want: []string{"v0.9.4"},
		},
		{
			name:    "fork zip bundle matches the fork pattern",
			pattern: "OptiScaler-NR-*.zip",
			setup: func(t *testing.T, bundleDir string) {
				writeFile(t, filepath.Join(bundleDir, "v0.8.92", "OptiScaler-NR-v0.8.92.zip"))
				writeFile(t, filepath.Join(bundleDir, "v0.8.92", "OptiScaler-NR-v0.8.92-SHA256SUMS.txt"))
			},
			want: []string{"v0.8.92"},
		},
		{
			name:    "version dir with only non-matching files is excluded",
			pattern: "Optiscaler_*.7z",
			setup: func(t *testing.T, bundleDir string) {
				writeFile(t, filepath.Join(bundleDir, "v0.9.4", ".download-123"))
				writeFile(t, filepath.Join(bundleDir, "v0.9.4", "notes.txt"))
			},
			want: nil,
		},
		{
			name:    "regular file named like a version is excluded",
			pattern: "Optiscaler_*.7z",
			setup: func(t *testing.T, bundleDir string) {
				writeFile(t, filepath.Join(bundleDir, "v0.9.4"))
			},
			want: nil,
		},
		{
			name:    "multiple versions sorted newest first (numeric, not lexicographic)",
			pattern: "Optiscaler_*.7z",
			setup: func(t *testing.T, bundleDir string) {
				writeFile(t, filepath.Join(bundleDir, "v0.9.4", "Optiscaler_a.7z"))
				writeFile(t, filepath.Join(bundleDir, "v0.10.0", "Optiscaler_b.7z"))
				writeFile(t, filepath.Join(bundleDir, "v0.9.10", "Optiscaler_c.7z"))
			},
			want: []string{"v0.10.0", "v0.9.10", "v0.9.4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundleDir := filepath.Join(t.TempDir(), "bundle")
			tt.setup(t, bundleDir)
			got := CachedVersions(bundleDir, tt.pattern)
			t.Logf("CachedVersions(%q, %q) = %v", bundleDir, tt.pattern, got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CachedVersions(%q, %q) = %v, want %v", bundleDir, tt.pattern, got, tt.want)
			}
		})
	}
}
