package steam

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const testSearchJSON = `[
  {"appid":"1245620","name":"ELDEN RING","icon":"abc","logo":"def"},
  {"appid":"977950","name":"A Different Game","icon":"","logo":""}
]`

// newTestClient returns a Client pointed at srv with an isolated cache dir.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "0.5.0")
}

func TestSearchApps_ParsesResults(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, testSearchJSON)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	appid, name, err := c.SearchApps(context.Background(), "elden ring")
	if err != nil {
		t.Fatalf("SearchApps: %v", err)
	}
	if appid != "1245620" {
		t.Errorf("appid = %q, want %q (first result wins)", appid, "1245620")
	}
	if name != "ELDEN RING" {
		t.Errorf("name = %q, want %q", name, "ELDEN RING")
	}
	if gotPath != "/actions/SearchApps/elden%20ring" {
		t.Errorf("request path = %q, want /actions/SearchApps/elden%%20ring", gotPath)
	}
	t.Logf("query=%q resolved appid=%s name=%q path=%s", "elden ring", appid, name, gotPath)
}

func TestSearchApps_RejectsImplausibleMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"appid":"1","name":"Something Else"}]`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _, err := c.SearchApps(context.Background(), "zzz nothing")
	if !errors.Is(err, ErrNoMatch) {
		t.Errorf("err = %v, want ErrNoMatch (result name %q is not a plausible match for the query)", err, "Something Else")
	}
	t.Logf("implausible match rejected: %v", err)
}

func TestSearchApps_EmptyResultsIsNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _, err := c.SearchApps(context.Background(), "anything")
	if !errors.Is(err, ErrNoMatch) {
		t.Errorf("err = %v, want ErrNoMatch on an empty result array", err)
	}
	t.Logf("empty results: %v", err)
}

func TestSearchApps_UsesCacheWithinTTL(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, testSearchJSON)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	for i := 0; i < 2; i++ {
		if _, _, err := c.SearchApps(context.Background(), "elden ring"); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits = %d, want 1 (second call must be served from the disk cache)", got)
	}
	t.Logf("two calls, %d server hit (cache hit within TTL)", hits.Load())
}

func TestSearchApps_RefetchesAfterTTL(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, testSearchJSON)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	now := time.Now()
	c.now = func() time.Time { return now }

	if _, _, err := c.SearchApps(context.Background(), "elden ring"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	now = now.Add(cacheTTL + time.Hour)
	if _, _, err := c.SearchApps(context.Background(), "elden ring"); err != nil {
		t.Fatalf("second call after TTL: %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("server hits = %d, want 2 (expired cache must refetch)", got)
	}
	t.Logf("cache expired after TTL: %d server hits", hits.Load())
}

func TestSearchApps_SetsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, testSearchJSON)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, _, err := c.SearchApps(context.Background(), "elden ring"); err != nil {
		t.Fatalf("SearchApps: %v", err)
	}
	if want := "optiscaler-manager/0.5.0"; gotUA != want {
		t.Errorf("User-Agent = %q, want %q", gotUA, want)
	}
	t.Logf("User-Agent: %q", gotUA)
}

func TestSearchApps_CooldownAfter429(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _, err := c.SearchApps(context.Background(), "elden ring")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("first call err = %v, want ErrRateLimited on HTTP 429", err)
	}
	_, _, err = c.SearchApps(context.Background(), "elden ring")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second call err = %v, want ErrRateLimited inside cooldown", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits = %d, want 1 (cooldown must not touch the network)", got)
	}
	t.Logf("429 -> cooldown: %d server hit across two calls", hits.Load())
}
