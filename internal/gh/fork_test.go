package gh

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// forkReleasesJSON mirrors the DLSSNR multipass fork's release shape: the
// bundle is a .zip next to a SHA256SUMS decoy, and the tag naming has no
// relation to upstream's.
const forkReleasesJSON = `[
  {
    "tag_name": "v0.8.92",
    "prerelease": false,
    "assets": [
      {"name": "OptiScaler-NR-v0.8.92-SHA256SUMS.txt", "browser_download_url": "https://example.invalid/sums.txt", "size": 92},
      {"name": "OptiScaler-NR-v0.8.92.zip", "browser_download_url": "https://example.invalid/bundle.zip", "size": 130482960}
    ]
  }
]`

// A fork client resolves against the fork's repository and selects assets
// with the fork's glob — the .zip bundle, not the checksum decoy.
func TestForkResolveUsesSlugAndPattern(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(forkReleasesJSON))
	}))
	defer srv.Close()

	c := NewForkWithBaseURL(srv.Client(), t.TempDir(), srv.URL,
		"jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass", "OptiScaler-NR-*.zip")
	got, _, err := c.Resolve(context.Background(), "latest")
	if err != nil {
		t.Fatalf("Resolve(latest) error: %v", err)
	}
	if got.AssetName != "OptiScaler-NR-v0.8.92.zip" {
		t.Errorf("AssetName = %q, want the .zip bundle (pattern must skip the SHA256SUMS decoy)", got.AssetName)
	}
	if got.Version != "v0.8.92" {
		t.Errorf("Version = %q, want v0.8.92", got.Version)
	}
	if !strings.Contains(gotPath, "/repos/jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass/releases") {
		t.Errorf("request path %q does not target the fork repository", gotPath)
	}
	t.Logf("fork resolve: path=%s asset=%s", gotPath, got.AssetName)
}

// Empty slug/pattern keep the upstream behavior, so the existing
// constructors and the OM_GH_BASE_URL test seam are untouched.
func TestForkDefaultsToUpstream(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(testReleasesJSON))
	}))
	defer srv.Close()

	c := NewForkWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "", "")
	got, _, err := c.Resolve(context.Background(), "latest")
	if err != nil {
		t.Fatalf("Resolve(latest) error: %v", err)
	}
	if got.AssetName != "Optiscaler_0.9.4-final.20260718._MM.7z" {
		t.Errorf("AssetName = %q, want the upstream .7z bundle", got.AssetName)
	}
	if !strings.Contains(gotPath, "/repos/optiscaler/OptiScaler/releases") {
		t.Errorf("request path %q does not target upstream", gotPath)
	}
}

// A pattern matching no asset fails loud, naming the pattern so a
// mistyped fork entry is diagnosable.
func TestForkPatternNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(forkReleasesJSON))
	}))
	defer srv.Close()

	c := NewForkWithBaseURL(srv.Client(), t.TempDir(), srv.URL,
		"jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass", "Optiscaler_*.7z")
	_, _, err := c.Resolve(context.Background(), "latest")
	if err == nil {
		t.Fatal("Resolve with a non-matching pattern expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Optiscaler_*.7z") {
		t.Errorf("error %q does not name the pattern", err)
	}
	t.Logf("no-match error: %v", err)
}

// The fork client's Download resolves the fork asset URL indexed at
// resolve time — the resolved .zip downloads like any bundle.
func TestForkDownload(t *testing.T) {
	payload := []byte("fake zip bundle bytes")
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			_, _ = w.Write(payload)
			return
		}
		_, _ = w.Write([]byte(strings.ReplaceAll(forkReleasesJSON, "https://example.invalid", srv.URL)))
	}))
	defer srv.Close()

	c := NewForkWithBaseURL(srv.Client(), t.TempDir(), srv.URL,
		"jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass", "OptiScaler-NR-*.zip")
	asset, _, err := c.Resolve(context.Background(), "latest")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	path, _, err := c.Download(context.Background(), asset, t.TempDir())
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !strings.HasSuffix(path, "OptiScaler-NR-v0.8.92.zip") {
		t.Errorf("downloaded path %q, want the fork zip name", path)
	}
}
