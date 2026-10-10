package umudb

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testServer returns an httptest server emulating umu_api.php: a hit
// for ("egs","Heather"), misses otherwise; calls counts requests.
func testServer(t *testing.T, calls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		q := r.URL.Query()
		if !strings.HasSuffix(r.URL.Path, "umu_api.php") {
			t.Errorf("path = %q, want .../umu_api.php", r.URL.Path)
		}
		if q.Get("store") == "egs" && q.Get("codename") == "Heather" {
			_, _ = io.WriteString(w, `[{"title":"Red Dead Redemption 2","umu_id":"umu-1174180"}]`)
			return
		}
		_, _ = io.WriteString(w, `[]`)
	}))
}

// TestLookupHitThenCache: a (store, codename) hit returns the umu ID;
// a second lookup of the same pair is served from the file cache
// without another HTTP request (issue 064).
func TestLookupHitThenCache(t *testing.T) {
	calls := 0
	srv := testServer(t, &calls)
	defer srv.Close()
	c := New(srv.Client(), t.TempDir(), "test-agent")
	c.baseURL = srv.URL + "/umu_api.php"

	id, err := c.Lookup(t.Context(), "egs", "Heather")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if id != "umu-1174180" {
		t.Errorf("umu ID = %q, want umu-1174180", id)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if _, err := c.Lookup(t.Context(), "egs", "Heather"); err != nil {
		t.Fatalf("cached Lookup: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d after cached lookup, want still 1", calls)
	}
}

// TestLookupMissCachesNoMatch: an empty result is ErrNoMatch, cached
// the same way — a game needing no fixes must not cost a request per
// launch. Failures degrade: the caller falls back, launch is never
// blocked.
func TestLookupMissCachesNoMatch(t *testing.T) {
	calls := 0
	srv := testServer(t, &calls)
	defer srv.Close()
	c := New(srv.Client(), t.TempDir(), "test-agent")
	c.baseURL = srv.URL + "/umu_api.php"

	_, err := c.Lookup(t.Context(), "egs", "NoSuchGame")
	if !errors.Is(err, ErrNoMatch) {
		t.Errorf("err = %v, want ErrNoMatch", err)
	}
	_, err = c.Lookup(t.Context(), "egs", "NoSuchGame")
	if !errors.Is(err, ErrNoMatch) {
		t.Errorf("cached err = %v, want ErrNoMatch", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (miss cached)", calls)
	}
}

// TestLookupNetworkErrorIsNotNoMatch: a transport failure is a plain
// error (retriable next launch), never cached as a miss.
func TestLookupNetworkErrorIsNotNoMatch(t *testing.T) {
	srv := testServer(t, new(int))
	srv.Close() // dead before any call
	c := New(srv.Client(), t.TempDir(), "test-agent")
	c.baseURL = srv.URL + "/umu_api.php"

	_, err := c.Lookup(t.Context(), "egs", "Heather")
	if err == nil {
		t.Fatal("dead server: want error, got nil")
	}
	if errors.Is(err, ErrNoMatch) {
		t.Errorf("err = %v, want a transport error, not ErrNoMatch", err)
	}
}

// TestLookupSendsUserAgent: the client identifies itself like the
// other online clients in this repo.
func TestLookupSendsUserAgent(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.UserAgent()
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()
	c := New(srv.Client(), t.TempDir(), "optiscaler-manager/test")
	c.baseURL = srv.URL + "/umu_api.php"
	_, _ = c.Lookup(t.Context(), "gog", "123")
	if ua != "optiscaler-manager/test" {
		t.Errorf("User-Agent = %q, want optiscaler-manager/test", ua)
	}
}
