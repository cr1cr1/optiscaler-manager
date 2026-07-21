package steam

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// newStoreTestClient points both API hosts at the test server.
func newStoreTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return NewWithBaseURLs(srv.Client(), t.TempDir(), srv.URL, srv.URL, "0.8.0")
}

func TestAppDetails_ResolvesNameAndDeveloper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/appdetails" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("appids") != "2322010" {
			t.Errorf("appids = %q, want 2322010", r.URL.Query().Get("appids"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"2322010":{"success":true,"data":{"name":"God of War Ragnarök","developers":["Santa Monica Studio"],"publishers":["Sony"]}}}`)
	}))
	defer srv.Close()

	c := newStoreTestClient(t, srv)
	name, dev, live, err := c.AppDetails(context.Background(), "2322010")
	if err != nil {
		t.Fatalf("AppDetails: %v", err)
	}
	if !live || name != "God of War Ragnarök" || dev != "Santa Monica Studio" {
		t.Errorf("got name=%q dev=%q live=%v", name, dev, live)
	}
	// Second call is served from the disk cache — no live request.
	name2, _, live2, err := c.AppDetails(context.Background(), "2322010")
	if err != nil {
		t.Fatalf("cached AppDetails: %v", err)
	}
	if live2 || name2 != name {
		t.Errorf("cache: name=%q live=%v, want cached %q", name2, live2, name)
	}
}

func TestAppDetails_FailureIsCachedNegative(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = fmt.Fprint(w, `{"480":{"success":false}}`)
	}))
	defer srv.Close()

	c := newStoreTestClient(t, srv)
	_, _, _, err := c.AppDetails(context.Background(), "480")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, want ErrNoMatch", err)
	}
	_, _, _, err = c.AppDetails(context.Background(), "480")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("cached err = %v, want ErrNoMatch", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server calls = %d, want 1 (negative cached)", got)
	}
}

func TestAppDetails_RateLimitCooldown(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newStoreTestClient(t, srv)
	if _, _, _, err := c.AppDetails(context.Background(), "730"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if _, _, _, err := c.AppDetails(context.Background(), "730"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("cooldown err = %v, want ErrRateLimited", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server calls = %d, want 1 (cooldown suppresses live retry)", got)
	}
}

func TestStoreSearch_ResolvesItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/storesearch/" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"total":2,"items":[
		  {"type":"app","name":"Black Myth: Wukong","id":2358720,"platforms":{"windows":true,"mac":false,"linux":false}},
		  {"type":"app","name":"Black Myth","id":999,"platforms":{"windows":true,"mac":true,"linux":true}}]}`)
	}))
	defer srv.Close()

	c := newStoreTestClient(t, srv)
	items, live, err := c.StoreSearch(context.Background(), "black myth wukong")
	if err != nil {
		t.Fatalf("StoreSearch: %v", err)
	}
	if !live || len(items) != 2 {
		t.Fatalf("got %d items live=%v", len(items), live)
	}
	if items[0].ID != "2358720" || items[0].Name != "Black Myth: Wukong" || !items[0].Windows || items[0].Linux {
		t.Errorf("items[0] = %+v", items[0])
	}
	items2, live2, err := c.StoreSearch(context.Background(), "black myth wukong")
	if err != nil {
		t.Fatalf("cached StoreSearch: %v", err)
	}
	if live2 || len(items2) != 2 {
		t.Errorf("cache: %d items live=%v, want 2 cached", len(items2), live2)
	}
}

func TestStoreSearch_EmptyIsCachedNegative(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = fmt.Fprint(w, `{"total":0,"items":[]}`)
	}))
	defer srv.Close()

	c := newStoreTestClient(t, srv)
	_, _, err := c.StoreSearch(context.Background(), "zzzz nothing")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, want ErrNoMatch", err)
	}
	_, _, err = c.StoreSearch(context.Background(), "zzzz nothing")
	if !errors.Is(err, ErrNoMatch) {
		t.Fatalf("cached err = %v, want ErrNoMatch", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server calls = %d, want 1 (negative cached)", got)
	}
}

func TestStoreSearch_RateLimitCooldown(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newStoreTestClient(t, srv)
	if _, _, err := c.StoreSearch(context.Background(), "x"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if _, _, err := c.StoreSearch(context.Background(), "x"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("cooldown err = %v, want ErrRateLimited", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server calls = %d, want 1 (cooldown suppresses live retry)", got)
	}
}
