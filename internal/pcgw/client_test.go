package pcgw

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "0.8.0")
}

func TestSearchTitle_ResolvesAndCaches(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Query().Get("action") != "opensearch" {
			t.Errorf("action = %q", r.URL.Query().Get("action"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `["pathologic",["Pathologic 2","Pathologic"],["desc1","desc2"],["u1","u2"]]`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	title, live, err := c.SearchTitle(context.Background(), "pathologic")
	if err != nil {
		t.Fatalf("SearchTitle: %v", err)
	}
	if !live || title != "Pathologic 2" {
		t.Errorf("got title=%q live=%v", title, live)
	}
	title2, live2, err := c.SearchTitle(context.Background(), "pathologic")
	if err != nil {
		t.Fatalf("cached: %v", err)
	}
	if live2 || title2 != title || atomic.LoadInt32(&calls) != 1 {
		t.Errorf("cache: title=%q live=%v calls=%d", title2, live2, calls)
	}
}

func TestSearchTitle_EmptyIsCachedNegative(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = fmt.Fprint(w, `["zzz",[],[],[]]`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, _, err := c.SearchTitle(context.Background(), "zzz"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, want ErrNoMatch", err)
	}
	if _, _, err := c.SearchTitle(context.Background(), "zzz"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("cached err = %v, want ErrNoMatch", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (negative cached)", got)
	}
}

func TestSearchTitle_RateLimitCooldown(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, _, err := c.SearchTitle(context.Background(), "x"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if _, _, err := c.SearchTitle(context.Background(), "x"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("cooldown err = %v, want ErrRateLimited", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (cooldown suppresses retry)", got)
	}
}

func TestTitleBySteamAppID_ResolvesAndCaches(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Query().Get("action") != "cargoquery" {
			t.Errorf("action = %q", r.URL.Query().Get("action"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"cargoquery":[{"title":{"Page":"Pathologic 2"}}]}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	title, live, err := c.TitleBySteamAppID(context.Background(), "1010300")
	if err != nil {
		t.Fatalf("TitleBySteamAppID: %v", err)
	}
	if !live || title != "Pathologic 2" {
		t.Errorf("got title=%q live=%v", title, live)
	}
	if _, live2, err := c.TitleBySteamAppID(context.Background(), "1010300"); err != nil || live2 {
		t.Errorf("cache: live=%v err=%v", live2, err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (cached)", got)
	}
}

func TestTitleBySteamAppID_EmptyIsCachedNegative(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = fmt.Fprint(w, `{"cargoquery":[]}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, _, err := c.TitleBySteamAppID(context.Background(), "999"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, want ErrNoMatch", err)
	}
	if _, _, err := c.TitleBySteamAppID(context.Background(), "999"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("cached err = %v, want ErrNoMatch", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (negative cached)", got)
	}
}
