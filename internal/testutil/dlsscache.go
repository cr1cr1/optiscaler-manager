package testutil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// SeedDLSSCacheDir plants a commit-keyed DLSS download-cache directory:
// each member written as given plus a manifest.json pinning the real
// SHA-256 of every member — the shape the dlss package's ensureCache
// leaves. (The dlss package's own tests keep a dedicated seeder: they must
// exercise its unexported record type, which this package cannot import
// without an import cycle.)
func SeedDLSSCacheDir(t *testing.T, cacheRoot, commit string, files map[string][]byte) {
	t.Helper()
	dir := filepath.Join(cacheRoot, "dlss", commit)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	digests := map[string]string{}
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		digests[name] = hex.EncodeToString(sum[:])
	}
	manifest, err := json.Marshal(map[string]map[string]string{"files": digests})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
}
