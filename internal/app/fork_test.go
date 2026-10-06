package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// Install with a fork slug resolves from the fork's repository, caches the
// bundle under the fork's namespace (never upstream's, so same-named tags
// cannot collide), and records the fork in the manifest.
func TestInstallForkNamespacesCacheAndManifest(t *testing.T) {
	f := newAppFakes(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/fork/OptiScaler-NR/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.8.92-fork","prerelease":false,"assets":[{"name":"Fork_test.7z","browser_download_url":%q,"size":100}]}]`, f.srv.URL+"/bundle")
	})
	forkSrv := httptest.NewServer(mux)
	defer forkSrv.Close()

	client := gh.NewForkWithBaseURL(nil, filepath.Join(t.TempDir(), "ghcache"), forkSrv.URL,
		"fork/OptiScaler-NR", "Fork_*.7z")
	m, err := Install(context.Background(), store.New(t.TempDir()), client, f.cacheDir, f.gameRoot,
		InstallOpts{ForkSlug: "fork/OptiScaler-NR"})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	cached := filepath.Join(settings.BundleCacheDir(f.cacheDir, "fork/OptiScaler-NR"), "v0.8.92-fork", "Fork_test.7z")
	if _, err := os.Stat(cached); err != nil {
		t.Errorf("fork bundle not cached at %s: %v", cached, err)
	}
	if m.Fork != "fork/OptiScaler-NR" {
		t.Errorf("manifest Fork = %q, want fork/OptiScaler-NR", m.Fork)
	}
	if m.Resolved.Version != "v0.8.92-fork" {
		t.Errorf("resolved version = %q, want v0.8.92-fork", m.Resolved.Version)
	}
}

// An empty fork slug is the upstream distribution: the bundle lands in
// the upstream namespace and the manifest records the upstream slug.
func TestInstallEmptyForkIsUpstream(t *testing.T) {
	f := newAppFakes(t)
	m, err := Install(context.Background(), f.st, f.client, f.cacheDir, f.gameRoot, InstallOpts{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	cached := filepath.Join(settings.BundleCacheDir(f.cacheDir, settings.DefaultForkSlug), "v0.9.4-test", "Optiscaler_test.7z")
	if _, err := os.Stat(cached); err != nil {
		t.Errorf("upstream bundle not cached at %s: %v", cached, err)
	}
	if m.Fork != settings.DefaultForkSlug {
		t.Errorf("manifest Fork = %q, want %q", m.Fork, settings.DefaultForkSlug)
	}
}
