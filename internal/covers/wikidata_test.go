package covers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/wikidata"
)

// wikiForCoversTest wires the Wikidata endpoints of a fake server:
// entity search, claims, and the Commons file redirect.
type wikiForCovers struct {
	label   string
	file    string
	apiHits int
}

func (f *wikiForCovers) handle(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/w/api.php":
			f.apiHits++
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Query().Get("action") {
			case "wbsearchentities":
				if f.label == "" {
					_, _ = fmt.Fprint(w, `{"search":[]}`)
					return
				}
				fmt.Fprintf(w, `{"search":[{"id":"Q1","label":%q}]}`, f.label)
			case "wbgetentities":
				fmt.Fprintf(w, `{"entities":{"Q1":{"claims":{"P18":[{"mainsnak":{"datavalue":{"value":%q}}}]}}}}`, f.file)
			}
		case strings.HasPrefix(r.URL.Path, "/wiki/Special:FilePath/"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(wideJPEG(t))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// Games with no Steam or PCGW presence (console titles via emulator)
// still get box art: Wikidata/Commons is the keyless last source, and
// the 2:3 aspect invariant holds on whatever Commons serves (issue 030).
func TestCoverWikidataWhenNoSteamNoPCGW(t *testing.T) {
	wiki := &wikiForCovers{label: "The Legend of Zelda: Breath of the Wild", file: "BotW Boxart.jpg"}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	mux.HandleFunc("/", wiki.handle(t))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.Wikidata = wikidata.NewWithBases(srv.Client(), t.TempDir(), srv.URL, srv.URL, "test")

	p, err := c.Cover(context.Background(), "", "The Legend of Zelda - Breath of the Wild")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(p), "wd_") {
		t.Errorf("path = %q, want a wd_-namespaced cache file", p)
	}
	img := decodeImg(t, p)
	b := img.Bounds()
	if d := 3*b.Dx() - 2*b.Dy(); d < -b.Dx()/33 || d > b.Dx()/33 {
		t.Errorf("aspect %dx%d violates the 2:3 invariant", b.Dx(), b.Dy())
	}
}

// A Wikidata miss is cached: the second lookup serves the placeholder
// without re-hitting the API.
func TestCoverWikidataMissCached(t *testing.T) {
	wiki := &wikiForCovers{} // no label → empty search
	mux := http.NewServeMux()
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	mux.HandleFunc("/", wiki.handle(t))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cacheDir := t.TempDir()
	c := NewWithBase(srv.Client(), cacheDir, srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")
	c.Wikidata = wikidata.NewWithBases(srv.Client(), t.TempDir(), srv.URL, srv.URL, "test")

	for i := 0; i < 2; i++ {
		p, err := c.Cover(context.Background(), "", "Nonexistent Game XYZZY")
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if !strings.HasSuffix(p, "_placeholder.png") {
			t.Fatalf("path = %q, want the placeholder", p)
		}
	}
	if wiki.apiHits != 1 {
		t.Errorf("api hits = %d, want 1 (the miss must be cached)", wiki.apiHits)
	}
}
