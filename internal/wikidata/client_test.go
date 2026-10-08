package wikidata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeWiki answers wbsearchentities/wbgetentities from its tables and
// counts API hits so cache behavior is observable.
type fakeWiki struct {
	entities map[string]string // lowercased search term → wbsearchentities payload
	claims   map[string]string // entity id → wbgetentities payload
	hits     int
}

func (f *fakeWiki) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/w/api.php" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f.hits++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("action") {
		case "wbsearchentities":
			term := strings.ToLower(r.URL.Query().Get("search"))
			if payload, ok := f.entities[term]; ok {
				_, _ = fmt.Fprint(w, payload)
				return
			}
			_, _ = fmt.Fprint(w, `{"search":[]}`)
		case "wbgetentities":
			id := r.URL.Query().Get("ids")
			if payload, ok := f.claims[id]; ok {
				_, _ = fmt.Fprint(w, payload)
				return
			}
			fmt.Fprintf(w, `{"entities":{%q:{"claims":{}}}}`, id)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The entity whose label (or alias) scores best against the title wins;
// its P18 image is the cover file.
func TestSearchCoverFilePicksBestEntity(t *testing.T) {
	f := &fakeWiki{
		entities: map[string]string{
			"riven": `{"search":[
			  {"id":"Q2","label":"Riven: The Sequel to Myst"},
			  {"id":"Q1","label":"Riven"}]}`,
		},
		claims: map[string]string{
			"Q1": `{"entities":{"Q1":{"claims":{"P18":[{"mainsnak":{"datavalue":{"value":"Riven Cover.jpg"}}}]}}}}`,
		},
	}
	srv := f.server(t)
	c := NewWithBases(srv.Client(), t.TempDir(), srv.URL, srv.URL, "test")

	file, live, err := c.SearchCoverFile(context.Background(), "Riven")
	if err != nil || !live {
		t.Fatalf("SearchCoverFile = %q, live=%v, %v", file, live, err)
	}
	if file != "Riven Cover.jpg" {
		t.Errorf("file = %q, want the exact-match entity's P18", file)
	}
}

// No acceptable entity (or no P18) is a stable answer: cached as a
// negative, later calls never re-hit the API.
func TestSearchCoverFileNegativeCached(t *testing.T) {
	f := &fakeWiki{entities: map[string]string{}, claims: map[string]string{}}
	srv := f.server(t)
	c := NewWithBases(srv.Client(), t.TempDir(), srv.URL, srv.URL, "test")

	_, _, err := c.SearchCoverFile(context.Background(), "Nonexistent Game XYZZY")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, want ErrNoMatch", err)
	}
	hits := f.hits
	_, _, err = c.SearchCoverFile(context.Background(), "Nonexistent Game XYZZY")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("second call err = %v, want cached ErrNoMatch", err)
	}
	if f.hits != hits {
		t.Errorf("hits grew %d → %d: the negative was not cached", hits, f.hits)
	}
}

// An entity with no P18 image is a miss, and an unrelated label set is
// never accepted.
func TestSearchCoverFileRejectsUnrelated(t *testing.T) {
	f := &fakeWiki{
		entities: map[string]string{
			"frostpunk": `{"search":[{"id":"Q9","label":"Frostpunk 2"}]}`,
		},
		claims: map[string]string{},
	}
	srv := f.server(t)
	c := NewWithBases(srv.Client(), t.TempDir(), srv.URL, srv.URL, "test")

	if _, _, err := c.SearchCoverFile(context.Background(), "Frostpunk"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, want ErrNoMatch for the sequel trap", err)
	}
}

// Commons file URLs underscore spaces and carry the thumb width.
func TestFileURL(t *testing.T) {
	c := NewWithBases(nil, t.TempDir(), "https://api.example", "https://files.example", "test")
	got := c.FileURL("BotW Box Art.png", 600)
	want := "https://files.example/wiki/Special:FilePath/BotW_Box_Art.png?width=600"
	if got != want {
		t.Errorf("FileURL = %q, want %q", got, want)
	}
}
