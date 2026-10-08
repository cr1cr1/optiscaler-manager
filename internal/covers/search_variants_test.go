package covers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// searchFake answers storesearch only for the listed terms (lowercased)
// and counts every query, so tests can prove which variants were tried.
type searchFake struct {
	itemsByTerm map[string]string
	terms       []string
}

func (f *searchFake) mux(t *testing.T) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, r *http.Request) {
		term := r.URL.Query().Get("term")
		f.terms = append(f.terms, term)
		w.Header().Set("Content-Type", "application/json")
		if payload, ok := f.itemsByTerm[strings.ToLower(term)]; ok {
			_, _ = fmt.Fprint(w, payload)
			return
		}
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	mux.HandleFunc("/steam/apps/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(tinyPNG(t))
	})
	return mux
}

// Steam's storesearch answers ZERO items for titles with extra tokens
// ("Spelunky HD"): the lookup must retry with the gid-normalized form,
// which strips edition tokens — and still pick the exact match over the
// sequel (issue 030).
func TestSearchAppIDNormalizedVariant(t *testing.T) {
	f := &searchFake{itemsByTerm: map[string]string{
		"spelunky": `{"items":[
		  {"id":418530,"name":"Spelunky 2","platforms":{"windows":true}},
		  {"id":239350,"name":"Spelunky","platforms":{"windows":true}}]}`,
	}}
	srv := httptest.NewServer(f.mux(t))
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "", "Spelunky HD")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "239350.img") {
		t.Errorf("path = %q, want Spelunky's art (239350.img), not the sequel's", p)
	}
	if len(f.terms) == 0 || f.terms[0] != "Spelunky HD" {
		t.Errorf("terms = %v, want the raw title tried first", f.terms)
	}
	found := false
	for _, term := range f.terms {
		if strings.EqualFold(term, "spelunky") {
			found = true
		}
	}
	if !found {
		t.Errorf("terms = %v, want the normalized variant queried", f.terms)
	}
}

// Long junky subtitles ("Riven - The sequel to Myst") match nothing
// verbatim; the query truncates token-by-token until the store answers.
func TestSearchAppIDTruncatesSubtitle(t *testing.T) {
	f := &searchFake{itemsByTerm: map[string]string{
		"riven": `{"items":[
		  {"id":1712350,"name":"Riven","platforms":{"windows":true}},
		  {"id":909660,"name":"Vagrus - The Riven Realms","platforms":{"windows":true}}]}`,
	}}
	srv := httptest.NewServer(f.mux(t))
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "", "Riven - The sequel to Myst")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "1712350.img") {
		t.Errorf("path = %q, want Riven's art (1712350.img)", p)
	}
	if got := f.terms[len(f.terms)-1]; got != "riven" {
		t.Errorf("last query = %q, want the fully truncated variant %q (all: %v)", got, "riven", f.terms)
	}
}

// Truncation never degrades into sub-4-character queries: "the" must
// never hit the store.
func TestSearchAppIDTruncationFloor(t *testing.T) {
	f := &searchFake{itemsByTerm: map[string]string{}}
	srv := httptest.NewServer(f.mux(t))
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "", "The Legend of Zelda - Breath of the Wild")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "_placeholder.png") {
		t.Errorf("path = %q, want the placeholder", p)
	}
	for _, term := range f.terms {
		if len(term) < 4 {
			t.Errorf("query %q below the truncation floor (all: %v)", term, f.terms)
		}
	}
}

// Truncated franchise-root queries are dangerous: edition stripping
// normalizes "The Witcher: Enhanced Edition Director's Cut" to exactly
// "the witcher", an exact match for the truncated query — the numeral
// that actually identifies the game was truncated away. Binding requires
// digit-token equality with the ORIGINAL title; Witcher 1 lacks the "3"
// and is refused even when it is the only candidate (issue 033).
func TestSearchAppIDNumeralGuardRejectsWrongGame(t *testing.T) {
	f := &searchFake{itemsByTerm: map[string]string{
		"the witcher": `{"items":[
		  {"id":20900,"name":"The Witcher: Enhanced Edition Director's Cut","platforms":{"windows":true}}]}`,
	}}
	srv := httptest.NewServer(f.mux(t))
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "", "The Witcher 3: Wild Hunt - Game of the Year Edition")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "_placeholder.png") {
		t.Errorf("path = %q, want the placeholder — Witcher 1 (20900) lacks the 3 and must never bind", p)
	}
}

// The guard is corroborating evidence, not just a veto: when the
// original title's numerals match the item's, a weak truncated query
// ("the witcher") MAY bind the right game ("The Witcher 3: Wild Hunt")
// — the numeral is what makes the franchise root unambiguous.
func TestSearchAppIDNumeralCorroboratesTruncatedQuery(t *testing.T) {
	f := &searchFake{itemsByTerm: map[string]string{
		"the witcher": `{"items":[
		  {"id":20900,"name":"The Witcher: Enhanced Edition Director's Cut","platforms":{"windows":true}},
		  {"id":292030,"name":"The Witcher® 3: Wild Hunt","platforms":{"windows":true}}]}`,
	}}
	srv := httptest.NewServer(f.mux(t))
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "", "The Witcher 3: Wild Hunt - Game of the Year Edition")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "292030.img") {
		t.Errorf("path = %q, want Witcher 3's art (292030.img) — numerals corroborate the truncated query", p)
	}
}

// Same rule, shorter tail: "cyberpunk" + matching 2077 binds
// Cyberpunk 2077.
func TestSearchAppIDNumeralMatchBinds(t *testing.T) {
	f := &searchFake{itemsByTerm: map[string]string{
		"cyberpunk": `{"items":[
		  {"id":1091500,"name":"Cyberpunk 2077","platforms":{"windows":true}}]}`,
	}}
	srv := httptest.NewServer(f.mux(t))
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "", "Cyberpunk 2077 GOTY")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "1091500.img") {
		t.Errorf("path = %q, want Cyberpunk 2077's art (numerals match)", p)
	}
}

// Without a numeral there is no corroboration: a franchise-root query
// must never subset-bind a different entry ("doom" ⊂ "Doom Eternal").
func TestSearchAppIDEmptyNumeralsNeverCorroborate(t *testing.T) {
	f := &searchFake{itemsByTerm: map[string]string{
		"doom": `{"items":[
		  {"id":20820,"name":"Doom 3","platforms":{"windows":true}},
		  {"id":782330,"name":"Doom Eternal","platforms":{"windows":true}}]}`,
	}}
	srv := httptest.NewServer(f.mux(t))
	t.Cleanup(srv.Close)
	c := NewWithBase(srv.Client(), t.TempDir(), srv.URL+"/steam/apps/%s/library_600x900.jpg", srv.URL+"/api/storesearch/")

	p, err := c.Cover(context.Background(), "", "Doom")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if !strings.HasSuffix(p, "_placeholder.png") {
		t.Errorf("path = %q, want the placeholder — no numeral corroboration, no bind", p)
	}
}
