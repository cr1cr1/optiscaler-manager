package sgdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return NewWithBaseURL(srv.Client(), t.TempDir(), "test-key", srv.URL, "0.8.0")
}

func TestGameBySteamAppID_ResolvesAndCaches(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		if r.URL.Path != "/games/steam/1867240" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"success":true,"data":{"id":55,"name":"WARDOGS"}}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	g, live, err := c.GameBySteamAppID(context.Background(), "1867240")
	if err != nil {
		t.Fatalf("GameBySteamAppID: %v", err)
	}
	if !live || g.ID != 55 || g.Name != "WARDOGS" {
		t.Errorf("got %+v live=%v", g, live)
	}
	if _, live2, err := c.GameBySteamAppID(context.Background(), "1867240"); err != nil || live2 {
		t.Errorf("cache: live=%v err=%v", live2, err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (cached)", got)
	}
}

func TestGameBySteamAppID_NotFoundIsCachedNegative(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"success":false,"errors":["Not found"]}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, _, err := c.GameBySteamAppID(context.Background(), "1"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, want ErrNoMatch", err)
	}
	if _, _, err := c.GameBySteamAppID(context.Background(), "1"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("cached err = %v, want ErrNoMatch", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (negative cached)", got)
	}
}

func TestSearchGame_TopHitAndCaches(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if !strings.HasPrefix(r.URL.Path, "/search/autocomplete/") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"success":true,"data":[{"id":52,"name":"The Witcher 3: Wild Hunt"},{"id":99,"name":"The Witcher"}]}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	g, live, err := c.SearchGame(context.Background(), "the witcher 3 remastered")
	if err != nil {
		t.Fatalf("SearchGame: %v", err)
	}
	if !live || g.ID != 52 || g.Name != "The Witcher 3: Wild Hunt" {
		t.Errorf("got %+v live=%v", g, live)
	}
	if _, live2, err := c.SearchGame(context.Background(), "the witcher 3 remastered"); err != nil || live2 {
		t.Errorf("cache: live=%v err=%v", live2, err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (cached)", got)
	}
}

func TestSearchGame_EmptyIsCachedNegative(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = fmt.Fprint(w, `{"success":true,"data":[]}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, _, err := c.SearchGame(context.Background(), "zzz"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, want ErrNoMatch", err)
	}
	if _, _, err := c.SearchGame(context.Background(), "zzz"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("cached err = %v, want ErrNoMatch", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (negative cached)", got)
	}
}

func TestGridURL_FirstGridAndCaches(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/grids/game/55" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("dimensions"); got != "600x900" {
			t.Errorf("dimensions = %q, want 600x900 (poster)", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"success":true,"data":[{"id":1,"score":9,"url":"https://cdn2.steamgriddb.com/grid/best.png"},{"id":2,"score":3,"url":"https://cdn2.steamgriddb.com/grid/worse.png"}]}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	u, live, err := c.GridURL(context.Background(), 55)
	if err != nil {
		t.Fatalf("GridURL: %v", err)
	}
	if !live || u != "https://cdn2.steamgriddb.com/grid/best.png" {
		t.Errorf("got url=%q live=%v", u, live)
	}
	if _, live2, err := c.GridURL(context.Background(), 55); err != nil || live2 {
		t.Errorf("cache: live=%v err=%v", live2, err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (cached)", got)
	}
}

// A 200 with success:false (bad key, malformed request) is a live API
// error — never cached as a negative (the issue-024 pcgw lesson).
func TestSuccessFalseIsLiveErrorNotCached(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"success":false,"errors":["Invalid API key"]}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _, err := c.SearchGame(context.Background(), "x")
	if !errors.Is(err, ErrAPI) {
		t.Fatalf("err = %v, want ErrAPI", err)
	}
	if errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, must not masquerade as ErrNoMatch", err)
	}
	if _, _, err := c.SearchGame(context.Background(), "x"); !errors.Is(err, ErrAPI) {
		t.Fatalf("second err = %v, want ErrAPI", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (API errors are never cached)", got)
	}
}

// An unauthorized key is a live error, not a cacheable negative.
func TestUnauthorizedIsLiveError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _, err := c.GameBySteamAppID(context.Background(), "1")
	if !errors.Is(err, ErrAPI) {
		t.Fatalf("err = %v, want ErrAPI", err)
	}
	if _, _, err := c.GameBySteamAppID(context.Background(), "1"); !errors.Is(err, ErrAPI) {
		t.Fatalf("second err = %v, want ErrAPI", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (401 is never cached)", got)
	}
}
