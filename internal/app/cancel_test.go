package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstallCancelBeforeDownload_LeavesFSClean cancels before Install: no
// resolve, no download, no manifest, no game-dir or cache writes.
func TestInstallCancelBeforeDownload_LeavesFSClean(t *testing.T) {
	f := newAppFakes(t)
	before := snapshotDir(t, f.gameRoot)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Install(ctx, f.st, f.client, f.cacheDir, f.gameRoot, InstallOpts{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Install err = %v, want errors.Is context.Canceled", err)
	}

	if f.bundleHits != 0 {
		t.Errorf("bundle downloaded %d times despite pre-download cancel", f.bundleHits)
	}
	if after := snapshotDir(t, f.gameRoot); before != after {
		t.Errorf("game dir changed by cancelled install\nbefore:\n%s\nafter:\n%s", before, after)
	}
	manifests, lerr := f.st.List()
	if lerr != nil || len(manifests) != 0 {
		t.Errorf("manifests after cancel: %d (%v), want 0", len(manifests), lerr)
	}
	// No bundle must have landed in the versioned cache.
	if _, err := os.Stat(filepath.Join(f.cacheDir, "optiscaler")); !os.IsNotExist(err) {
		t.Error("bundle cache populated despite pre-download cancel")
	}
	t.Log("cancel before download: zero downloads, zero manifests, FS clean")
}

// snapshotDir renders path:sha256 lines for every file under dir (app-side
// twin of the installer test helper).
func snapshotDir(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		sum, err := fileSHA256(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		lines = append(lines, rel+":"+sum)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotDir: %v", err)
	}
	return strings.Join(lines, "\n")
}
