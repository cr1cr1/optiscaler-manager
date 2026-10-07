package pcgw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "0.8.0")
}

func TestSearchTitles_ReturnsAllHitsAndCaches(t *testing.T) {
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
	titles, live, err := c.SearchTitles(context.Background(), "pathologic")
	if err != nil {
		t.Fatalf("SearchTitles: %v", err)
	}
	if !live || len(titles) != 2 || titles[0] != "Pathologic 2" || titles[1] != "Pathologic" {
		t.Errorf("got titles=%v live=%v", titles, live)
	}
	titles2, live2, err := c.SearchTitles(context.Background(), "pathologic")
	if err != nil {
		t.Fatalf("cached: %v", err)
	}
	if live2 || len(titles2) != len(titles) || atomic.LoadInt32(&calls) != 1 {
		t.Errorf("cache: titles=%v live=%v calls=%d", titles2, live2, calls)
	}
}

func TestSearchTitles_EmptyIsCachedNegative(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = fmt.Fprint(w, `["zzz",[],[],[]]`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, _, err := c.SearchTitles(context.Background(), "zzz"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, want ErrNoMatch", err)
	}
	if _, _, err := c.SearchTitles(context.Background(), "zzz"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("cached err = %v, want ErrNoMatch", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (negative cached)", got)
	}
}

func TestSearchTitles_RateLimitCooldown(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, _, err := c.SearchTitles(context.Background(), "x"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	if _, _, err := c.SearchTitles(context.Background(), "x"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("cooldown err = %v, want ErrRateLimited", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (cooldown suppresses retry)", got)
	}
}

// The wiki's Cargo API now answers anonymous queries with HTTP 200 plus an
// error envelope (permissiondenied). That is a live failure — it must NOT be
// decoded as "empty results" and cached as a 30-day negative (issue 024).
func TestAPIErrorEnvelopeIsLiveAndNeverCached(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"error":{"code":"permissiondenied","info":"You don't have permission to run arbitrary Cargo queries."}}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _, err := c.CoverFile(context.Background(), "End of Abyss")
	if !errors.Is(err, ErrAPI) {
		t.Fatalf("err = %v, want ErrAPI", err)
	}
	if errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, must not masquerade as ErrNoMatch", err)
	}
	if _, _, err := c.CoverFile(context.Background(), "End of Abyss"); !errors.Is(err, ErrAPI) {
		t.Fatalf("second err = %v, want ErrAPI", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls = %d, want 2 (API errors are never cached as negatives)", got)
	}
}

// Pre-repair cache files (legacy key hash, no "v2:" prefix) may hold
// negatives poisoned by the error-envelope bug; the client must ignore them
// and go live instead (issue 024 clean break).
func TestLegacyCacheEntriesAreIgnored(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `["end of abyss",["End of Abyss"],[""],["u"]]`)
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	// The legacy naming scheme, replicated for the test: sha256 of
	// kind+":"+lower(key), no version prefix.
	sum := sha256.Sum256([]byte("title:" + strings.ToLower(strings.TrimSpace("End of Abyss"))))
	legacy := filepath.Join(cacheDir, "title_"+hex.EncodeToString(sum[:])[:16]+".json")
	legacyEntry := fmt.Sprintf(`{"fetched_at":%q,"no_match":true}`, time.Now().UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(legacy, []byte(legacyEntry), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewWithBaseURL(srv.Client(), cacheDir, srv.URL, "0.8.0")
	titles, live, err := c.SearchTitles(context.Background(), "End of Abyss")
	if err != nil {
		t.Fatalf("SearchTitles: %v (legacy poisoned negative must be ignored)", err)
	}
	if !live || len(titles) != 1 || titles[0] != "End of Abyss" {
		t.Errorf("got titles=%v live=%v", titles, live)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (legacy entry ignored, went live)", got)
	}
}

func TestCoverFile_ParsesWikitextInfobox(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		q := r.URL.Query()
		if q.Get("action") != "query" || q.Get("prop") != "revisions" {
			t.Errorf("action=%q prop=%q, want wikitext revisions query", q.Get("action"), q.Get("prop"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"query":{"pages":[{"title":"End of Abyss","revisions":[{"slots":{"main":{"content":"{{Infobox game\n|cover        = End of Abyss cover.jpg\n|steam appid  = \n}}\n"}}}]}]}}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	name, live, err := c.CoverFile(context.Background(), "End of Abyss")
	if err != nil {
		t.Fatalf("CoverFile: %v", err)
	}
	if !live || name != "End of Abyss cover.jpg" {
		t.Errorf("got name=%q live=%v", name, live)
	}
	if _, live2, err := c.CoverFile(context.Background(), "End of Abyss"); err != nil || live2 {
		t.Errorf("cache: live=%v err=%v", live2, err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (cached)", got)
	}
}

// A page with no cover line in its infobox is a true negative and IS cached.
func TestCoverFile_NoCoverLineIsCachedNegative(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"query":{"pages":[{"title":"X","revisions":[{"slots":{"main":{"content":"{{Infobox game\n|cover = \n|steam appid = 123\n}}\n"}}}]}]}}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, _, err := c.CoverFile(context.Background(), "X"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("err = %v, want ErrNoMatch", err)
	}
	if _, _, err := c.CoverFile(context.Background(), "X"); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("cached err = %v, want ErrNoMatch", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (true negative cached)", got)
	}
}

func TestImageThumbURL_ResolvesAndCaches(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"query":{"pages":{"5719":{"title":"File:From Dust cover.png","imageinfo":[{"thumburl":"https://thumbnails.pcgamingwiki.com/3/3b/From_Dust_cover.png/600px-From_Dust_cover.png","thumbwidth":600}]}}}}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	u, live, err := c.ImageThumbURL(context.Background(), "From_Dust_cover.png", 600)
	if err != nil {
		t.Fatalf("ImageThumbURL: %v", err)
	}
	if !live || u != "https://thumbnails.pcgamingwiki.com/3/3b/From_Dust_cover.png/600px-From_Dust_cover.png" {
		t.Errorf("got url=%q live=%v", u, live)
	}
	if _, live2, err := c.ImageThumbURL(context.Background(), "From_Dust_cover.png", 600); err != nil || live2 {
		t.Errorf("cache: live=%v err=%v", live2, err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (cached)", got)
	}
}
