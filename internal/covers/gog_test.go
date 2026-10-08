package covers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/gogdb"
	"github.com/cr1cr1/optiscaler-manager/internal/wikidata"
)

// gogForCovers wires the GOG catalog endpoints of a fake server: catalog
// search by like-term and image hosting for coverVertical URLs.
type gogForCovers struct {
	byTerm     map[string]string // lowercased like-term → catalog payload (may reference {IMG})
	apiHits    int
	imgServed  string
	imgServedN int
}

func (f *gogForCovers) mux(t *testing.T, srv **httptest.Server) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/catalog", func(w http.ResponseWriter, r *http.Request) {
		f.apiHits++
		w.Header().Set("Content-Type", "application/json")
		term := strings.ToLower(strings.TrimPrefix(r.URL.Query().Get("query"), "like:"))
		if payload, ok := f.byTerm[term]; ok {
			_, _ = fmt.Fprint(w, strings.ReplaceAll(payload, "{IMG}", (*srv).URL+"/img"))
			return
		}
		_, _ = fmt.Fprint(w, `{"products":[]}`)
	})
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) {
		f.imgServed = r.URL.Path
		f.imgServedN++
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(wideJPEG(t))
	})
	return mux
}

// GOG vertical store art covers games when Steam's search finds nothing
// and PCGW has no page. The catalog's like: ranking degrades with junk
// tokens exactly like Steam's, so the same query variants walk down to a
// binding term — and ties keep the earlier hit, so Riven's 1997 original
// outranks the remake (issue 034).
func TestCoverGOGFallbackBindsOriginalRiven(t *testing.T) {
	var srv *httptest.Server
	gog := &gogForCovers{byTerm: map[string]string{
		"riven": `{"products":[
		  {"id":"1","productType":"game","title":"Riven (1997)","coverVertical":"{IMG}/riven1997.jpg"},
		  {"id":"2","productType":"game","title":"Riven","coverVertical":"{IMG}/riven2024.jpg"}]}`,
	}}
	mux := gog.mux(t, &srv)
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.GOG = gogdb.NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "test")

	p, err := c.Cover(context.Background(), "", "Riven - The sequel to Myst")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(p), "gog_") {
		t.Errorf("path = %q, want a gog_-namespaced cache file", p)
	}
	if gog.imgServed != "/img/riven1997.jpg" {
		t.Errorf("served %q, want the 1997 original's art (tie keeps the earlier hit)", gog.imgServed)
	}
	img := decodeImg(t, p)
	b := img.Bounds()
	if d := 3*b.Dx() - 2*b.Dy(); d < -b.Dx()/33 || d > b.Dx()/33 {
		t.Errorf("aspect %dx%d violates the 2:3 invariant", b.Dx(), b.Dy())
	}
}

// The numeral rule governs GOG binding too: Witcher 1 is vetoed for a
// Witcher 3 title, and the matching {3} corroborates the truncated
// "the witcher" query into Witcher 3's art (issue 034 + 033).
func TestCoverGOGNumeralRule(t *testing.T) {
	var srv *httptest.Server
	gog := &gogForCovers{byTerm: map[string]string{
		"the witcher": `{"products":[
		  {"id":"1","productType":"game","title":"The Witcher: Enhanced Edition Director's Cut","coverVertical":"{IMG}/witcher1.jpg"},
		  {"id":"2","productType":"game","title":"The Witcher 3: Wild Hunt — Remastered","coverVertical":"{IMG}/witcher3.jpg"}]}`,
	}}
	mux := gog.mux(t, &srv)
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.GOG = gogdb.NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "test")

	p, err := c.Cover(context.Background(), "", "The Witcher 3: Wild Hunt - Game of the Year Edition")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if gog.imgServed != "/img/witcher3.jpg" {
		t.Errorf("served %q (path %q), want Witcher 3's art — Witcher 1 vetoed, {3} corroborates", gog.imgServed, p)
	}
}

// The veto alone means no bind: with only Witcher 1 on offer, the cover
// falls through to the placeholder rather than binding the wrong game.
func TestCoverGOGVetoFallsThrough(t *testing.T) {
	var srv *httptest.Server
	gog := &gogForCovers{byTerm: map[string]string{
		"the witcher": `{"products":[
		  {"id":"1","productType":"game","title":"The Witcher: Enhanced Edition Director's Cut","coverVertical":"{IMG}/witcher1.jpg"}]}`,
	}}
	mux := gog.mux(t, &srv)
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.GOG = gogdb.NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "test")

	p, err := c.Cover(context.Background(), "", "The Witcher 3: Wild Hunt - Game of the Year Edition")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "_placeholder.png") {
		t.Errorf("path = %q, want the placeholder (Witcher 1 vetoed)", p)
	}
	if gog.imgServedN != 0 {
		t.Errorf("served %d images, want 0 (no fetch for a vetoed bind)", gog.imgServedN)
	}
}

// Chain order: GOG's store art beats the Wikidata/Commons fallback.
func TestCoverGOGPreferredOverWikidata(t *testing.T) {
	var srv *httptest.Server
	gog := &gogForCovers{byTerm: map[string]string{
		"the legend of zelda breath of the wild": `{"products":[
		  {"id":"1","productType":"game","title":"The Legend of Zelda: Breath of the Wild","coverVertical":"{IMG}/zelda.jpg"}]}`,
	}}
	mux := gog.mux(t, &srv)
	wiki := &wikiForCovers{label: "The Legend of Zelda: Breath of the Wild", file: "BotW Boxart.jpg"}
	mux.Handle("/w/api.php", wiki.handle(t))
	mux.HandleFunc("/wiki/Special:FilePath/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(wideJPEG(t))
	})
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.GOG = gogdb.NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "test")
	c.Wikidata = wikidata.NewWithBases(srv.Client(), t.TempDir(), srv.URL, srv.URL, "test")

	p, err := c.Cover(context.Background(), "", "The Legend of Zelda - Breath of the Wild")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(p), "gog_") {
		t.Errorf("path = %q, want the GOG art (it outranks Wikidata)", p)
	}
	if wiki.apiHits != 0 {
		t.Errorf("wikidata api hits = %d, want 0 (GOG bound first)", wiki.apiHits)
	}
}
