package gh

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testReleasesJSON = `[
  {
    "tag_name": "0.9.5-nightly",
    "prerelease": true,
    "assets": [
      {"name": "Optiscaler_0.9.5-nightly.7z", "browser_download_url": "https://example.invalid/nightly.7z", "size": 100}
    ]
  },
  {
    "tag_name": "0.9.4",
    "prerelease": false,
    "assets": [
      {"name": "Optiscaler_0.9.4-final.20260718._MM.7z", "browser_download_url": "https://example.invalid/bundle.7z", "size": 42000000},
      {"name": "Optiscaler_0.9.4-final.20260718._MM.zip", "browser_download_url": "https://example.invalid/bundle.zip", "size": 43000000},
      {"name": "README.txt", "browser_download_url": "https://example.invalid/readme.txt", "size": 1234}
    ]
  },
  {
    "tag_name": "0.9.3",
    "prerelease": false,
    "assets": [
      {"name": "Optiscaler_0.9.3-final.7z", "browser_download_url": "https://example.invalid/old.7z", "size": 41000000}
    ]
  }
]`

// newTestClient returns a Client pointed at srv with an isolated cache dir.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := New(srv.Client(), t.TempDir())
	c.baseURL = srv.URL
	return c
}

func TestResolveReleaseMatchesAssetGlob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testReleasesJSON))
	}))
	defer srv.Close()

	ctx := context.Background()

	t.Run("latest resolves first non-prerelease .7z via glob", func(t *testing.T) {
		c := newTestClient(t, srv)
		got, fromCache, err := c.Resolve(ctx, "latest")
		if err != nil {
			t.Fatalf("Resolve(latest) error: %v", err)
		}
		if fromCache {
			t.Errorf("fromCache = true, want false on fresh fetch")
		}
		if got.Version != "0.9.4" {
			t.Errorf("Version = %q, want %q (prerelease must be skipped)", got.Version, "0.9.4")
		}
		wantAsset := "Optiscaler_0.9.4-final.20260718._MM.7z"
		if got.AssetName != wantAsset {
			t.Errorf("AssetName = %q, want %q (glob Optiscaler_*.7z must ignore .zip and README decoys)", got.AssetName, wantAsset)
		}
		if got.SHA256 != "" {
			t.Errorf("SHA256 = %q, want empty (GitHub API provides no digest; filled at download time)", got.SHA256)
		}
		t.Logf("requested=latest resolved version=%s asset=%s", got.Version, got.AssetName)
	})

	t.Run("exact tag resolves", func(t *testing.T) {
		c := newTestClient(t, srv)
		got, _, err := c.Resolve(ctx, "0.9.3")
		if err != nil {
			t.Fatalf("Resolve(0.9.3) error: %v", err)
		}
		if got.Version != "0.9.3" || got.AssetName != "Optiscaler_0.9.3-final.7z" {
			t.Errorf("got %+v, want version 0.9.3 asset Optiscaler_0.9.3-final.7z", got)
		}
		t.Logf("requested=0.9.3 resolved version=%s asset=%s", got.Version, got.AssetName)
	})

	t.Run("missing tag fails loud naming the requested value", func(t *testing.T) {
		c := newTestClient(t, srv)
		_, _, err := c.Resolve(ctx, "9.9.9")
		if err == nil {
			t.Fatal("Resolve(9.9.9) expected error, got nil")
		}
		if !strings.Contains(err.Error(), "9.9.9") {
			t.Errorf("error %q does not name requested value %q", err, "9.9.9")
		}
		t.Logf("missing tag error: %v", err)
	})

	t.Run("release without matching asset errors", func(t *testing.T) {
		srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[{"tag_name":"1.0","prerelease":false,"assets":[{"name":"README.txt","browser_download_url":"https://example.invalid/x","size":1}]}]`))
		}))
		defer srv2.Close()
		c := newTestClient(t, srv2)
		_, _, err := c.Resolve(ctx, "latest")
		if err == nil {
			t.Fatal("expected error when no Optiscaler_*.7z asset exists")
		}
		t.Logf("no-asset error: %v", err)
	})
}

func TestRateLimitCooldownServesCachedReleases(t *testing.T) {
	ctx := context.Background()
	cacheDir := t.TempDir()

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testReleasesJSON))
	}))
	defer srv.Close()

	t.Run("second call inside cooldown serves cache without hitting server", func(t *testing.T) {
		c := New(srv.Client(), cacheDir)
		c.baseURL = srv.URL

		first, fromCache, err := c.Resolve(ctx, "latest")
		if err != nil {
			t.Fatalf("first Resolve error: %v", err)
		}
		if fromCache {
			t.Fatal("first call must not be from cache")
		}
		t.Logf("first call: version=%s asset=%s hits=%d", first.Version, first.AssetName, hits)

		second, fromCache, err := c.Resolve(ctx, "latest")
		if err != nil {
			t.Fatalf("second Resolve inside cooldown error: %v", err)
		}
		if !fromCache {
			t.Error("second call inside cooldown must report fromCache=true")
		}
		if second != first {
			t.Errorf("cached resolution %+v differs from fresh %+v", second, first)
		}
		if hits != 1 {
			t.Errorf("server hits = %d, want 1 (cooldown must not touch network)", hits)
		}
		t.Logf("second call served from cache: version=%s asset=%s hits=%d", second.Version, second.AssetName, hits)
	})

	t.Run("rate-limited response with cache serves cache", func(t *testing.T) {
		limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
		}))
		defer limited.Close()

		// Fresh client, expired cooldown (zero), but cache present from prior run.
		c := New(limited.Client(), cacheDir)
		c.baseURL = limited.URL
		// Expire the cooldown so the network is actually attempted.
		c.now = func() time.Time { return time.Now().Add(cooldown + time.Minute) }

		got, fromCache, err := c.Resolve(ctx, "latest")
		if err != nil {
			t.Fatalf("Resolve after 403 with cache error: %v", err)
		}
		if !fromCache {
			t.Error("rate-limited response with cache must report fromCache=true")
		}
		if got.Version != "0.9.4" {
			t.Errorf("Version = %q, want 0.9.4 from cache", got.Version)
		}
		t.Logf("403 fallback: version=%s fromCache=%v", got.Version, fromCache)
	})

	t.Run("cooldown without cache returns ErrRateLimited", func(t *testing.T) {
		c := New(srv.Client(), t.TempDir())
		c.baseURL = srv.URL
		// Simulate a persisted recent attempt with no releases.json.
		if err := c.writeCooldown(time.Now()); err != nil {
			t.Fatalf("writeCooldown: %v", err)
		}
		_, _, err := c.Resolve(ctx, "latest")
		if !errors.Is(err, ErrRateLimited) {
			t.Errorf("err = %v, want ErrRateLimited", err)
		}
		t.Logf("no-cache cooldown error: %v", err)
	})

	t.Run("rate-limited response without cache returns ErrRateLimited", func(t *testing.T) {
		limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer limited.Close()
		c := New(limited.Client(), t.TempDir())
		c.baseURL = limited.URL
		_, _, err := c.Resolve(ctx, "latest")
		if !errors.Is(err, ErrRateLimited) {
			t.Errorf("err = %v, want ErrRateLimited on 429 without cache", err)
		}
		t.Logf("429 no-cache error: %v", err)
	})
}

func TestRequestedVsResolvedRecordedSeparately(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testReleasesJSON))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	requested := "0.9.4"
	resolved, fromCache, err := c.Resolve(context.Background(), requested)
	if err != nil {
		t.Fatalf("Resolve(%q) error: %v", requested, err)
	}

	// The caller pairs the requested string with the returned ResolvedAsset;
	// Resolve never mutates or embeds the request. Assert they are distinct
	// values that travel separately (docs/safety.md manifest model).
	if resolved.Version != "0.9.4" {
		t.Errorf("resolved.Version = %q, want 0.9.4", resolved.Version)
	}
	if resolved.AssetName == requested {
		t.Errorf("resolved asset %q must differ from requested %q", resolved.AssetName, requested)
	}
	if fromCache {
		t.Error("fromCache = true on fresh fetch")
	}
	t.Logf("requested=%q resolved={version:%s asset:%s sha256:%q} fromCache=%v",
		requested, resolved.Version, resolved.AssetName, resolved.SHA256, fromCache)
}

func TestDownloadComputesSHA256(t *testing.T) {
	payload := []byte("fake 7z bundle bytes for hashing")
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".7z") {
			_, _ = w.Write(payload)
			return
		}
		_, _ = w.Write([]byte(strings.ReplaceAll(testReleasesJSON, "https://example.invalid", srv.URL)))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	asset, _, err := c.Resolve(context.Background(), "latest")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	destDir := t.TempDir()
	path, sha, err := c.Download(context.Background(), asset, destDir)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if filepath.Base(path) != asset.AssetName {
		t.Errorf("path base = %q, want %q", filepath.Base(path), asset.AssetName)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("downloaded bytes differ from payload")
	}
	if len(sha) != 64 {
		t.Errorf("sha256 hex length = %d, want 64", len(sha))
	}
	t.Logf("downloaded %s (%d bytes) sha256=%s", path, len(got), sha)
}
