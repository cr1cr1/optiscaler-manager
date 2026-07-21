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

	"github.com/cr1cr1/optiscaler-manager/internal/pcgw"
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

// When the portrait (600x900) art is missing but the hero banner exists,
// the hero is used before falling back to the placeholder.
func TestCoverHeroFallbackWhenNoPortrait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "library_600x900.jpg") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, "library_hero.jpg") {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(tinyPNG(t))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "3768760", "007 First Light")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if strings.HasSuffix(p, "_placeholder.png") {
		t.Errorf("path = %q, want the hero banner, not the placeholder", p)
	}
	if !strings.HasSuffix(p, "3768760.img") {
		t.Errorf("path = %q, want the appid-keyed image", p)
	}
}

// The store search must not bind a cover to an implausible first hit:
// "AC Shadows" must never fetch the Shadows on the Vatican cover.
func TestSearchAppIDRejectsImplausibleFirstHit(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[
		  {"id":378630,"name":"Shadows on the Vatican - Act II: Wrath","platforms":{"windows":true}},
		  {"id":999,"name":"Incredible Dracula: Academy of Shadows","platforms":{"windows":true}}]}`)
	})
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		// The wrong game's art exists — an unscored first-hit picks it up.
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(tinyPNG(t))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "", "AC Shadows")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "_placeholder.png") {
		t.Errorf("path = %q, want placeholder (no plausible match), not a wrong cover", p)
	}
}

// A correct-but-not-first item wins over an unrelated first hit.
func TestSearchAppIDPicksScoredOverFirst(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[
		  {"id":1,"name":"Totally Unrelated Thing","platforms":{"windows":true}},
		  {"id":2358720,"name":"Black Myth: Wukong","platforms":{"windows":true}}]}`)
	})
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "2358720") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(tinyPNG(t))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "", "Black Myth Wukong")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "2358720.img") {
		t.Errorf("path = %q, want the correct game's art", p)
	}
}

// A 404'd appid is remembered (short TTL) so every rescan does not
// re-hit the CDN for artless games.
func TestCoverMissIsCachedBriefly(t *testing.T) {
	hits := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	if _, err := c.Cover(context.Background(), "3768760", "007 First Light"); err != nil {
		t.Fatal(err)
	}
	first := hits
	if _, err := c.Cover(context.Background(), "3768760", "007 First Light"); err != nil {
		t.Fatal(err)
	}
	if hits != first {
		t.Errorf("CDN hits = %d then %d, want no refetch while the miss marker is fresh", first, hits)
	}
}

// PCGW box art (portrait) wins over the Steam hero banner (landscape)
// when the portrait poster is missing — users want poster-like art.
func TestCoverPCGWPreferredOverHero(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "library_hero.jpg") {
			_, _ = w.Write([]byte("HEROART"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	var srv *httptest.Server
	mux.HandleFunc("/w/api.php", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("action") {
		case "cargoquery":
			if strings.Contains(r.URL.RawQuery, "HOLDS") {
				_, _ = fmt.Fprint(w, `{"cargoquery":[{"title":{"Page":"From Dust"}}]}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"cargoquery":[{"title":{"Page":"From Dust","Cover":"From_Dust_cover.png"}}]}`)
		case "query":
			fmt.Fprintf(w, `{"query":{"pages":{"1":{"imageinfo":[{"thumburl":%q,"thumbwidth":600}]}}}}`, srv.URL+"/thumb/from_dust.png")
		default:
			_, _ = fmt.Fprint(w, `["From Dust",["From Dust"],[""],["https://x"]]`)
		}
	})
	mux.HandleFunc("/thumb/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("PCGWART"))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.PCGW = pcgwForCoversTest(t, srv)

	p, err := c.Cover(context.Background(), "33460", "From Dust")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "PCGWART" {
		t.Errorf("cover = %q, want PCGW box art over the hero banner", string(data))
	}
}

// With no Steam match at all, the wiki's title search finds the box art.
func TestCoverPCGWByTitleWhenNoSteam(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	mux.HandleFunc("/w/api.php", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("action") {
		case "opensearch":
			_, _ = fmt.Fprint(w, `["Alan Wake 2",["Alan Wake II"],[""],["https://x"]]`)
		case "cargoquery":
			_, _ = fmt.Fprint(w, `{"cargoquery":[{"title":{"Page":"Alan Wake II","Cover":"Alan_Wake_II_cover.jpg"}}]}`)
		case "query":
			fmt.Fprintf(w, `{"query":{"pages":{"1":{"imageinfo":[{"thumburl":%q,"thumbwidth":600}]}}}}`, srv.URL+"/thumb/aw2.jpg")
		}
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/thumb/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("PCGWART"))
	})
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.PCGW = pcgwForCoversTest(t, srv)

	p, err := c.Cover(context.Background(), "", "Alan Wake 2")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "PCGWART" {
		t.Errorf("cover = %q, want the wiki box art", string(data))
	}
}

func pcgwForCoversTest(t *testing.T, srv *httptest.Server) *pcgw.Client {
	t.Helper()
	return pcgw.NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "test")
}

// The wiki's thumbnail host rejects requests without a descriptive
// User-Agent (403 even for Go's default UA): every fetch must carry one.
func TestFetchSendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.UserAgent()
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("img"))
	}))
	defer srv.Close()
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	if _, err := c.Cover(context.Background(), "33460", "x"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotUA, "optiscaler-manager") {
		t.Errorf("User-Agent = %q, want a descriptive optiscaler-manager UA (wiki policy)", gotUA)
	}
}
