package covers

import (
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// writePNG writes a solid w×h PNG.
func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// Issue 042: Steam's CDN now serves library_600x900.jpg at HALF resolution
// (300x450); the true 600x900 art lives at library_600x900_2x.jpg. The
// fetch chain must try the _2x variant first so the detail-pane poster
// renders from the highest-quality source available, not an upscale of a
// 300px-wide file.

// artServer serves distinguishable bytes per variant and records the
// request order.
func artServer(t *testing.T, serve2x bool) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var order []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		order = append(order, r.URL.Path)
		mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "library_600x900_2x.jpg"):
			if !serve2x {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte("HIDPI-ART"))
		case strings.HasSuffix(r.URL.Path, "library_600x900.jpg"):
			_, _ = w.Write([]byte("LODPI-ART"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &order
}

func TestCoverPrefers2xArt(t *testing.T) {
	srv, order := artServer(t, true)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "440", "Whatever")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read cached cover: %v", err)
	}
	if string(b) != "HIDPI-ART" {
		t.Errorf("cached cover = %q, want the _2x variant's bytes", b)
	}
	reqs := *order
	if len(reqs) == 0 || !strings.HasSuffix(reqs[0], "library_600x900_2x.jpg") {
		t.Errorf("first art request = %v, want the _2x variant before the 1x", reqs)
	}
}

func TestCoverFallsBackTo1xWhen2xMissing(t *testing.T) {
	srv, _ := artServer(t, false)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "440", "Whatever")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read cached cover: %v", err)
	}
	if string(b) != "LODPI-ART" {
		t.Errorf("cached cover = %q, want the 1x fallback's bytes", b)
	}
}

// The cache accumulated 300x450 files under the old single-variant fetch:
// a marker-gated migration drops narrow CDN-named covers so they re-fetch
// at the _2x resolution. Other sources' files (sgdb_/pcgw_/gog_) keep
// their own sizes.
func TestHiresMigrationDropsNarrowCDNCovers(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "440.img"), 300, 450)         // narrow CDN file: dropped
	writePNG(t, filepath.Join(dir, "222.img"), 600, 900)         // already hi-res: kept
	writePNG(t, filepath.Join(dir, "sgdb_deadbeef.img"), 64, 96) // other source: kept
	// Pretend the sharpen migration already ran so only the hires sweep is
	// under test.
	_ = os.WriteFile(filepath.Join(dir, sharpenedMarker), nil, 0o600)

	New(nil, dir)

	if _, err := os.Stat(filepath.Join(dir, "440.img")); !os.IsNotExist(err) {
		t.Error("narrow CDN cover survived the hires migration")
	}
	for _, kept := range []string{"222.img", "sgdb_deadbeef.img"} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Errorf("%s was dropped: %v", kept, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, hiresMarker)); err != nil {
		t.Error("hires marker not written")
	}
}
