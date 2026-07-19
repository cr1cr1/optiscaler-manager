package covers

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// fakeCDN serves covers and store-search responses like Steam's services.
type fakeCDN struct {
	srv        *httptest.Server
	coverHits  int
	searchHits int
	knownAppID string
	knownName  string
	coverBytes []byte
	failCovers bool
}

func newFakeCDN(t *testing.T) *fakeCDN {
	t.Helper()
	f := &fakeCDN{knownAppID: "1091500", knownName: "Cyberpunk 2077"}
	f.coverBytes = tinyPNG(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		f.coverHits++
		if f.failCovers || !strings.Contains(r.URL.Path, f.knownAppID) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(f.coverBytes)
	})
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		f.searchHits++
		term := strings.ToLower(r.URL.Query().Get("term"))
		if f.failCovers || !strings.Contains(strings.ToLower(f.knownName), term) {
			_, _ = fmt.Fprint(w, `{"items":[]}`)
			return
		}
		fmt.Fprintf(w, `{"items":[{"id":%s,"name":%q}]}`, f.knownAppID, f.knownName)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{20, 20, 40, 255})
		}
	}
	var sb strings.Builder
	w := &builderWriter{&sb}
	if err := png.Encode(w, img); err != nil {
		t.Fatal(err)
	}
	return []byte(sb.String())
}

type builderWriter struct{ b *strings.Builder }

func (w *builderWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

func TestCoverFetchedAndCached(t *testing.T) {
	f := newFakeCDN(t)
	c := New(nil, t.TempDir())
	c.cdnBase = f.srv.URL + "/steam/apps/%s/library_600x900.jpg"
	c.searchBase = f.srv.URL + "/api/storesearch/"

	p1, err := c.Cover(context.Background(), "1091500", "Cyberpunk 2077")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if p1 == "" {
		t.Fatal("expected a cached cover path, got empty")
	}
	data, err := os.ReadFile(p1)
	if err != nil || len(data) == 0 {
		t.Fatalf("cached cover unreadable: %v", err)
	}
	if f.coverHits != 1 {
		t.Fatalf("expected 1 CDN hit, got %d", f.coverHits)
	}

	p2, err := c.Cover(context.Background(), "1091500", "Cyberpunk 2077")
	if err != nil {
		t.Fatalf("Cover (cached): %v", err)
	}
	if p2 != p1 {
		t.Errorf("cached call returned different path %q vs %q", p2, p1)
	}
	if f.coverHits != 1 {
		t.Errorf("cached call hit the network again (%d hits)", f.coverHits)
	}
	t.Logf("cover cached at %s after 1 hit", p1)
}

func TestStoreSearchFallback(t *testing.T) {
	f := newFakeCDN(t)
	c := New(nil, t.TempDir())
	c.cdnBase = f.srv.URL + "/steam/apps/%s/library_600x900.jpg"
	c.searchBase = f.srv.URL + "/api/storesearch/"

	// Unknown appid: direct CDN misses, store search resolves by name.
	p, err := c.Cover(context.Background(), "999999", "cyberpunk")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if p == "" {
		t.Fatal("expected cover via store-search fallback, got empty")
	}
	if f.searchHits == 0 {
		t.Error("store search was never consulted")
	}
	t.Logf("fallback resolved via search: %s", p)
}

func TestCoverMissUsesPlaceholder(t *testing.T) {
	f := newFakeCDN(t)
	f.failCovers = true
	c := New(nil, t.TempDir())
	c.cdnBase = f.srv.URL + "/steam/apps/%s/library_600x900.jpg"
	c.searchBase = f.srv.URL + "/api/storesearch/"

	p, err := c.Cover(context.Background(), "0", "nonexistent game")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if p == "" {
		t.Fatal("expected placeholder path, got empty")
	}
	r, err := os.Open(p)
	if err != nil {
		t.Fatalf("placeholder missing: %v", err)
	}
	defer func() { _ = r.Close() }()
	if _, err := png.Decode(r); err != nil {
		t.Fatalf("placeholder is not a valid PNG: %v", err)
	}
	t.Logf("placeholder at %s", p)
}

func TestCoverCacheKeySanitizesAppID(t *testing.T) {
	f := newFakeCDN(t)
	c := New(nil, t.TempDir())
	c.cdnBase = f.srv.URL + "/steam/apps/%s/library_600x900.jpg"
	c.searchBase = f.srv.URL + "/api/storesearch/"

	// AppIDs come from manifests and should be digits; anything weird must
	// not escape the cache dir.
	p, err := c.Cover(context.Background(), "../../evil", "x")
	if err == nil && strings.Contains(p, "..") {
		t.Fatalf("cache path escaped: %q", p)
	}
}
