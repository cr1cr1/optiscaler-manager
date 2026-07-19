package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

type appFakes struct {
	srv        *httptest.Server
	bundleHits int
	gameRoot   string
	bin        string
	st         *store.Store
	client     *gh.Client
	cacheDir   string
	fixtureSHA string
}

func newAppFakes(t *testing.T) *appFakes {
	t.Helper()
	f := &appFakes{}
	fixture, err := os.ReadFile(filepath.Join("..", "installer", "testdata", "bundle.7z"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(fixture)
	f.fixtureSHA = hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/optiscaler/OptiScaler/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"tag_name":"v0.9.4-test","prerelease":false,"assets":[{"name":"Optiscaler_test.7z","browser_download_url":%q,"size":100}]}]`, f.srv.URL+"/bundle")
	})
	mux.HandleFunc("/bundle", func(w http.ResponseWriter, r *http.Request) {
		f.bundleHits++
		_, _ = w.Write(fixture)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)

	root := t.TempDir()
	f.cacheDir = filepath.Join(root, "cache")
	f.st = store.New(root)
	f.client = gh.NewWithBaseURL(nil, filepath.Join(root, "ghcache"), f.srv.URL)

	f.gameRoot = t.TempDir()
	f.bin = filepath.Join(f.gameRoot, "bin")
	if err := os.MkdirAll(f.bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.bin, "game.exe"), []byte("GAME"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// seedCache places the fixture bundle at the versioned cache path.
func (f *appFakes) seedCache(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(f.cacheDir, "optiscaler", "v0.9.4-test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join("..", "installer", "testdata", "bundle.7z")
	dst := filepath.Join(dir, "Optiscaler_test.7z")
	if err := copyFileHelper(src, dst); err != nil {
		t.Fatal(err)
	}
	return dst
}

func copyFileHelper(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func TestInstallUsesCachedBundleFirst(t *testing.T) {
	f := newAppFakes(t)
	cached := f.seedCache(t)

	m, err := Install(context.Background(), f.st, f.client, f.cacheDir, f.gameRoot, InstallOpts{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if f.bundleHits != 0 {
		t.Errorf("bundle downloaded %d times despite valid cache", f.bundleHits)
	}
	if m.Resolved.SHA256 != f.fixtureSHA {
		t.Errorf("manifest sha %q != cached bundle sha %q", m.Resolved.SHA256, f.fixtureSHA)
	}
	if _, err := os.Stat(filepath.Join(f.bin, "dxgi.dll")); err != nil {
		t.Errorf("install did not complete from cache: %v", err)
	}
	t.Logf("cache hit, zero downloads, manifest sha ok (%s)", cached)
}

func TestInstallDownloadsWhenCacheMissing(t *testing.T) {
	f := newAppFakes(t)

	m, err := Install(context.Background(), f.st, f.client, f.cacheDir, f.gameRoot, InstallOpts{})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if f.bundleHits != 1 {
		t.Fatalf("expected 1 download, got %d", f.bundleHits)
	}
	cached := filepath.Join(f.cacheDir, "optiscaler", "v0.9.4-test", "Optiscaler_test.7z")
	if _, err := os.Stat(cached); err != nil {
		t.Fatalf("downloaded bundle not cached at %s: %v", cached, err)
	}

	// Second install (after uninstall) must reuse the cached bundle even when
	// release info is served stale from the gh cooldown (consent given).
	if _, err := Uninstall(context.Background(), f.st, f.gameRoot); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	m2, err := Install(context.Background(), f.st, f.client, f.cacheDir, f.gameRoot, InstallOpts{AllowCached: true})
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if f.bundleHits != 1 {
		t.Errorf("second install re-downloaded (hits=%d)", f.bundleHits)
	}
	if m2.Resolved.SHA256 != m.Resolved.SHA256 {
		t.Errorf("sha changed between installs")
	}
	t.Log("download-once-then-cache verified")
}

var _ = domain.StatusCommitted
