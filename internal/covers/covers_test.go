package covers

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/pcgw"
	"github.com/cr1cr1/optiscaler-manager/internal/sgdb"
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
	p, err := c.Cover(context.Background(), "999999", "cyberpunk 2077")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if p == "" {
		t.Fatal("expected cover via store-search fallback, got empty")
	}
	if !strings.HasSuffix(p, f.knownAppID+".img") {
		t.Errorf("path = %q, want the searched game's art %q", p, f.knownAppID+".img")
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

// A known-artless appid (recent .miss marker) skips the CDN retry but must
// NOT suppress the title-search fallback: for manual games the appid guess
// can be wrong while the title still resolves.
func TestCoverRecentMissStillSearchesByTitle(t *testing.T) {
	f := newFakeCDN(t)
	cacheDir := t.TempDir()
	c := New(nil, cacheDir)
	c.cdnBase = f.srv.URL + "/steam/apps/%s/library_600x900.jpg"
	c.searchBase = f.srv.URL + "/api/storesearch/"

	// Seed the negative marker for the wrong appid.
	miss := filepath.Join(cacheDir, "999999.miss")
	if err := os.WriteFile(miss, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := c.Cover(context.Background(), "999999", "cyberpunk 2077")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, f.knownAppID+".img") {
		t.Errorf("path = %q, want the title-search hit %q (miss marker must not block the fallback)", p, f.knownAppID+".img")
	}
	if f.searchHits == 0 {
		t.Error("store search never ran behind a recent appid miss")
	}
	t.Logf("title fallback fired despite the miss marker: %s", p)
}

// When several candidates pass the acceptance gate, the best-probability
// one wins (PC build outranks non-PC at equal title score), not the first.
func TestSearchAppIDPicksBestScored(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[
		  {"id":111,"name":"Cyberpunk","platforms":{"windows":false}},
		  {"id":222,"name":"Cyberpunk","platforms":{"windows":true}}]}`)
	})
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "222") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(tinyPNG(t))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "", "Cyberpunk")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "222.img") {
		t.Errorf("path = %q, want the PC candidate's art (best score), not the first hit", p)
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
		case "query":
			if r.URL.Query().Get("prop") == "revisions" {
				_, _ = fmt.Fprint(w, `{"query":{"pages":[{"title":"From Dust","revisions":[{"slots":{"main":{"content":"{{Infobox game\n|cover = From_Dust_cover.png\n}}\n"}}}]}]}}`)
				return
			}
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
		case "query":
			if r.URL.Query().Get("prop") == "revisions" {
				_, _ = fmt.Fprint(w, `{"query":{"pages":[{"title":"Alan Wake II","revisions":[{"slots":{"main":{"content":"{{Infobox game\n|cover = Alan_Wake_II_cover.jpg\n}}\n"}}}]}]}}`)
				return
			}
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

// sgdbForCoversTest returns a SteamGridDB client against the fake server.
func sgdbForCoversTest(t *testing.T, srv *httptest.Server) *sgdb.Client {
	t.Helper()
	return sgdb.NewWithBaseURL(srv.Client(), t.TempDir(), "test-key", srv.URL, "test")
}

// fakeSGDB wires the SGDB endpoints of a fake server: the steam-appid
// mapping, autocomplete, and grid list for one known game.
type fakeSGDB struct {
	steamAppID       string
	gameID           int64
	gameName         string
	gridHits         int
	autocompleteHits int
}

func (f *fakeSGDB) handle(srv **httptest.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/games/steam/"):
			if strings.HasSuffix(r.URL.Path, "/"+f.steamAppID) {
				fmt.Fprintf(w, `{"success":true,"data":{"id":%d,"name":%q}}`, f.gameID, f.gameName)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"success":false,"errors":["Not found"]}`)
		case strings.HasPrefix(r.URL.Path, "/search/autocomplete/"):
			f.autocompleteHits++
			fmt.Fprintf(w, `{"success":true,"data":[{"id":%d,"name":%q}]}`, f.gameID, f.gameName)
		case strings.HasPrefix(r.URL.Path, "/grids/game/"):
			f.gridHits++
			fmt.Fprintf(w, `{"success":true,"data":[{"id":1,"score":9,"url":%q}]}`, (*srv).URL+"/grid/x.png")
		}
	}
}

// SGDB grid art fills the gap when Steam's CDN has no art for the appid
// (unreleased/asset-less games like WARDOGS) — and wins over the
// landscape hero banner, which is the last resort (issue 024).
func TestCoverSGDBWhenCDNHasNoArt(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	f := fakeSGDB{steamAppID: "1867240", gameID: 55, gameName: "WARDOGS"}
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "library_hero.jpg") {
			_, _ = w.Write([]byte("HEROART"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/games/steam/", f.handle(&srv))
	mux.HandleFunc("/search/autocomplete/", f.handle(&srv))
	mux.HandleFunc("/grids/game/", f.handle(&srv))
	mux.HandleFunc("/grid/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("SGDBART"))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.SGDB = sgdbForCoversTest(t, srv)

	p, err := c.Cover(context.Background(), "1867240", "WARDOGS")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "SGDBART" {
		t.Errorf("cover = %q, want the SGDB grid, not the hero banner", string(data))
	}
	if !strings.HasSuffix(p, "1867240.img") {
		t.Errorf("path = %q, want the appid-keyed image", p)
	}
}

// The SGDB name search is alias-aware: its top hit binds under a scoped
// subset rule even when the strict scorer rejects it — a folder named
// "The Witcher 3 Remastered" (no subtitle) resolves to The Witcher 3:
// Wild Hunt's grid (issue 024).
func TestCoverSGDBNameSubsetAccepts(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	f := fakeSGDB{steamAppID: "0", gameID: 52, gameName: "The Witcher 3: Wild Hunt"}
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	mux.HandleFunc("/games/steam/", f.handle(&srv))
	mux.HandleFunc("/search/autocomplete/", f.handle(&srv))
	mux.HandleFunc("/grids/game/", f.handle(&srv))
	mux.HandleFunc("/grid/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("SGDBART"))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.SGDB = sgdbForCoversTest(t, srv)

	p, err := c.Cover(context.Background(), "", "The Witcher 3 Remastered")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "SGDBART" {
		t.Errorf("cover = %q, want the SGDB grid via the subset rule", string(data))
	}
}

// The subset rule refuses when the hit adds a numeral: "Frostpunk" must
// not pick up Frostpunk 2's art (the same guard as the strict scorer).
func TestCoverSGDBNameRejectsNewNumeral(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	f := fakeSGDB{steamAppID: "0", gameID: 77, gameName: "Frostpunk 2"}
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	mux.HandleFunc("/games/steam/", f.handle(&srv))
	mux.HandleFunc("/search/autocomplete/", f.handle(&srv))
	mux.HandleFunc("/grids/game/", f.handle(&srv))
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.SGDB = sgdbForCoversTest(t, srv)

	p, err := c.Cover(context.Background(), "", "Frostpunk")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "_placeholder.png") {
		t.Errorf("path = %q, want placeholder (Frostpunk must not bind Frostpunk 2's art)", p)
	}
	if f.gridHits != 0 {
		t.Errorf("grids fetched %d times despite the numeral guard", f.gridHits)
	}
}

// A configured-but-unknown appid falls through to the alias-aware name
// search: SGDB has no /games/steam mapping (404) but autocomplete still
// resolves the title (issue 024).
func TestCoverSGDBAppIDUnknownFallsToName(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	f := fakeSGDB{steamAppID: "0", gameID: 55, gameName: "WARDOGS"}
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/games/steam/", f.handle(&srv))
	mux.HandleFunc("/search/autocomplete/", f.handle(&srv))
	mux.HandleFunc("/grids/game/", f.handle(&srv))
	mux.HandleFunc("/grid/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("SGDBART"))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.SGDB = sgdbForCoversTest(t, srv)

	p, err := c.Cover(context.Background(), "999999", "WARDOGS")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "SGDBART" {
		t.Errorf("cover = %q, want the SGDB grid via the name fallback", string(data))
	}
	if f.autocompleteHits == 0 {
		t.Error("autocomplete never ran behind the unknown appid")
	}
}

// The wiki's opensearch ranks loosely: an unacceptable first hit must not
// bury an exact-match later hit — every candidate is scored and the best
// accepted one binds (issue 024).
func TestCoverPCGWPicksBestScoredHit(t *testing.T) {
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/w/api.php", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("action") {
		case "opensearch":
			_, _ = fmt.Fprint(w, `["The Witcher 3 Remastered",["The Witcher 3: Wild Hunt: Remastered","The Witcher 3 Remastered"],["",""],["u1","u2"]]`)
		case "query":
			if r.URL.Query().Get("prop") == "revisions" {
				switch r.URL.Query().Get("titles") {
				case "The Witcher 3 Remastered":
					_, _ = fmt.Fprint(w, `{"query":{"pages":[{"title":"The Witcher 3 Remastered","revisions":[{"slots":{"main":{"content":"{{Infobox game\n|cover = W3R_exact_cover.jpg\n}}\n"}}}]}]}}`)
				default:
					_, _ = fmt.Fprint(w, `{"query":{"pages":[{"title":"X","revisions":[{"slots":{"main":{"content":"{{Infobox game\n|cover = WRONG_cover.jpg\n}}\n"}}}]}]}}`)
				}
				return
			}
			if strings.Contains(r.URL.RawQuery, "W3R_exact_cover.jpg") {
				fmt.Fprintf(w, `{"query":{"pages":{"1":{"imageinfo":[{"thumburl":%q,"thumbwidth":600}]}}}}`, srv.URL+"/thumb/w3r.jpg")
				return
			}
			fmt.Fprintf(w, `{"query":{"pages":{"1":{"imageinfo":[{"thumburl":%q,"thumbwidth":600}]}}}}`, srv.URL+"/thumb/wrong.jpg")
		}
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/thumb/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "w3r") {
			_, _ = w.Write([]byte("PCGWART"))
			return
		}
		_, _ = w.Write([]byte("WRONGART"))
	})
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.PCGW = pcgwForCoversTest(t, srv)

	p, err := c.Cover(context.Background(), "", "The Witcher 3 Remastered")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "PCGWART" {
		t.Errorf("cover = %q, want the exact-match page's art, not the first hit's", string(data))
	}
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

// wideJPEG builds a 300×90 landscape image with a red left band, a green
// center band, and a blue right band — the shape of a Steam hero banner,
// with the center (where hero art puts the logo) clearly marked.
func wideJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 300, 90))
	for y := 0; y < 90; y++ {
		for x := 0; x < 300; x++ {
			switch {
			case x < 120:
				img.Set(x, y, color.RGBA{200, 30, 30, 255})
			case x < 180:
				img.Set(x, y, color.RGBA{30, 200, 30, 255})
			default:
				img.Set(x, y, color.RGBA{30, 30, 200, 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// decodeImg reads and decodes a cached cover file (content-sniffed).
func decodeImg(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return img
}

// The aspect invariant (issue 025): a landscape hero fallback must be
// cached as a 2:3 portrait image — center-cropped, never stretched by
// the aspect-blind renderer downstream.
func TestCoverHeroIsCenterCroppedToPortrait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "library_hero.jpg") {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(wideJPEG(t))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "3768760", "007 First Light")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	img := decodeImg(t, p)
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if 3*w != 2*h {
		t.Errorf("cached hero is %dx%d (aspect %.3f), want exact 2:3 portrait", w, h, float64(w)/float64(h))
	}
	// The 300-wide source crops to its center 60px — the green band;
	// red (left) and blue (right) must be gone.
	r, g, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
	if g < 0x8000 || r > 0x4000 || b > 0x4000 {
		t.Errorf("center pixel = (%x,%x,%x), want the green center band (sides cropped)", r, g, b)
	}
}

// Already-2:3 art passes through byte-identical: normalization must never
// re-encode (and degrade) proper posters.
func TestCoverPortraitArtPassesThroughUntouched(t *testing.T) {
	f := newFakeCDN(t)
	c := New(nil, t.TempDir())
	c.cdnBase = f.srv.URL + "/steam/apps/%s/library_600x900.jpg"
	c.searchBase = f.srv.URL + "/api/storesearch/"

	p, err := c.Cover(context.Background(), "1091500", "Cyberpunk 2077")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, f.coverBytes) {
		t.Errorf("2:3 art was re-encoded (%d → %d bytes), want byte-identical passthrough", len(f.coverBytes), len(data))
	}
}

// A landscape image cached before the aspect invariant existed is
// scrubbed in place on read: legacy art self-heals without a cache wipe.
func TestCachedLegacyLandscapeScrubbedOnRead(t *testing.T) {
	f := newFakeCDN(t)
	cacheDir := t.TempDir()
	c := New(nil, cacheDir)
	c.cdnBase = f.srv.URL + "/steam/apps/%s/library_600x900.jpg"
	c.searchBase = f.srv.URL + "/api/storesearch/"

	cached := filepath.Join(cacheDir, "3768760.img")
	if err := os.WriteFile(cached, wideJPEG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := c.Cover(context.Background(), "3768760", "007 First Light")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if p != cached {
		t.Errorf("path = %q, want the cached file %q", p, cached)
	}
	img := decodeImg(t, cached)
	if w, h := img.Bounds().Dx(), img.Bounds().Dy(); 3*w != 2*h {
		t.Errorf("legacy file is %dx%d after read, want scrubbed to 2:3", w, h)
	}
}

// Non-image bytes in a cached file are a graceful no-op: no panic, no
// rewrite (the renderer already tolerates unloadable files).
func TestCachedJunkFileIsLeftAlone(t *testing.T) {
	f := newFakeCDN(t)
	cacheDir := t.TempDir()
	c := New(nil, cacheDir)
	c.cdnBase = f.srv.URL + "/steam/apps/%s/library_600x900.jpg"
	c.searchBase = f.srv.URL + "/api/storesearch/"

	cached := filepath.Join(cacheDir, "3768760.img")
	if err := os.WriteFile(cached, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := c.Cover(context.Background(), "3768760", "x")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "not an image" {
		t.Errorf("junk file rewritten to %q, want a graceful no-op", string(data))
	}
}
