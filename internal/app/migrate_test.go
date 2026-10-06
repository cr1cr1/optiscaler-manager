package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// The pre-fork cache layout (releases.json, cooldown.json, and
// optiscaler/<tag>/ bundle dirs at the cache root) migrates into the
// upstream fork's namespace; existing fork namespaces are untouched.
func TestMigrateLegacyBundleCache(t *testing.T) {
	cacheDir := t.TempDir()
	upstream := settings.BundleCacheDir(cacheDir, settings.DefaultForkSlug)

	// Legacy layout.
	writeFile(t, filepath.Join(cacheDir, "releases.json"))
	writeFile(t, filepath.Join(cacheDir, "cooldown.json"))
	writeFile(t, filepath.Join(cacheDir, "optiscaler", "v0.9.4", "Optiscaler_test.7z"))
	writeFile(t, filepath.Join(cacheDir, "optiscaler", "v0.9.3", "Optiscaler_old.7z"))
	// An existing fork namespace must not move.
	writeFile(t, filepath.Join(cacheDir, "optiscaler", "fork__OptiScaler-NR", "v0.8.92", "Fork_test.7z"))

	MigrateLegacyBundleCache(cacheDir)

	for _, p := range []string{
		filepath.Join(upstream, "releases.json"),
		filepath.Join(upstream, "cooldown.json"),
		filepath.Join(upstream, "v0.9.4", "Optiscaler_test.7z"),
		filepath.Join(upstream, "v0.9.3", "Optiscaler_old.7z"),
		filepath.Join(cacheDir, "optiscaler", "fork__OptiScaler-NR", "v0.8.92", "Fork_test.7z"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s after migration: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "releases.json")); !os.IsNotExist(err) {
		t.Error("legacy releases.json still at the cache root")
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "optiscaler", "v0.9.4")); !os.IsNotExist(err) {
		t.Error("legacy tag dir still at the old location")
	}
}

// A name collision (the upstream namespace already has the tag) leaves the
// legacy copy in place — the cache is regenerable, never destructively
// merged.
func TestMigrateLegacyBundleCacheCollisionKeepsLegacy(t *testing.T) {
	cacheDir := t.TempDir()
	upstream := settings.BundleCacheDir(cacheDir, settings.DefaultForkSlug)
	writeFile(t, filepath.Join(cacheDir, "optiscaler", "v0.9.4", "Optiscaler_legacy.7z"))
	writeFile(t, filepath.Join(upstream, "v0.9.4", "Optiscaler_new.7z"))

	MigrateLegacyBundleCache(cacheDir)

	if _, err := os.Stat(filepath.Join(cacheDir, "optiscaler", "v0.9.4", "Optiscaler_legacy.7z")); err != nil {
		t.Error("legacy tag dir vanished despite the collision")
	}
	if _, err := os.Stat(filepath.Join(upstream, "v0.9.4", "Optiscaler_new.7z")); err != nil {
		t.Error("existing upstream namespace content damaged")
	}
}

// A missing or empty cache dir is a no-op.
func TestMigrateLegacyBundleCacheNoop(t *testing.T) {
	MigrateLegacyBundleCache(filepath.Join(t.TempDir(), "absent"))
}
