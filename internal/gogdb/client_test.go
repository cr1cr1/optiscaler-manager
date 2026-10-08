package gogdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeCatalog answers /v1/catalog from its term table (keyed by the
// lowercased term after "like:") and counts hits for cache assertions.
type fakeCatalog struct {
	byTerm map[string]string
	hits   int
}

func (f *fakeCatalog) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f.hits++
		w.Header().Set("Content-Type", "application/json")
		term := strings.ToLower(strings.TrimPrefix(r.URL.Query().Get("query"), "like:"))
		if payload, ok := f.byTerm[term]; ok {
			_, _ = fmt.Fprint(w, payload)
			return
		}
		_, _ = fmt.Fprint(w, `{"products":[]}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Products parse with id/title/coverVertical; non-game entries (DLC,
// movies) are filtered out.
func TestSearchProductsParsesGamesOnly(t *testing.T) {
	f := &fakeCatalog{byTerm: map[string]string{
		"riven": `{"products":[
		  {"id":"1","productType":"game","title":"Riven (1997)","coverVertical":"https://img.example/riven.jpg"},
		  {"id":"2","productType":"dlc","title":"Riven Soundtrack","coverVertical":"https://img.example/ost.jpg"},
		  {"id":"3","productType":"movie","title":"Riven: The Movie","coverVertical":""}]}`,
	}}
	srv := f.server(t)
	c := NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "test")

	prods, live, err := c.SearchProducts(context.Background(), "Riven")
	if err != nil || !live {
		t.Fatalf("SearchProducts = %+v, live=%v, %v", prods, live, err)
	}
	if len(prods) != 1 {
		t.Fatalf("got %d products, want 1 (games only): %+v", len(prods), prods)
	}
	p := prods[0]
	if p.ID != "1" || p.Title != "Riven (1997)" || p.CoverVertical == "" {
		t.Errorf("product = %+v, want the parsed game entry", p)
	}
}

// An empty result is a stable answer: cached as a negative, later calls
// never re-hit the API.
func TestSearchProductsNegativeCached(t *testing.T) {
	f := &fakeCatalog{byTerm: map[string]string{}}
	srv := f.server(t)
	c := NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "test")

	_, _, err := c.SearchProducts(context.Background(), "Nonexistent XYZZY")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, want ErrNoMatch", err)
	}
	hits := f.hits
	if _, _, err := c.SearchProducts(context.Background(), "Nonexistent XYZZY"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("second call err = %v, want cached ErrNoMatch", err)
	}
	if f.hits != hits {
		t.Errorf("hits grew %d → %d: the negative was not cached", hits, f.hits)
	}
}

// 429/5xx are live failures with a cooldown, never cached negatives.
func TestSearchProductsRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	c := NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "test")

	if _, _, err := c.SearchProducts(context.Background(), "Riven"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if _, _, err := c.SearchProducts(context.Background(), "Riven"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("cooldown err = %v, want ErrRateLimited without a live call", err)
	}
}
