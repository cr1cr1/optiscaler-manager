package protondb

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

const testSummaryJSON = `{
  "tier": "gold",
  "confidence": "strong",
  "score": 82,
  "total": 1371,
  "bestReportedTier": "platinum",
  "trendingTier": "gold",
  "provisionalTier": "gold"
}`

// newTestClient returns a Client pointed at srv with an isolated cache dir.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return NewWithBaseURL(srv.Client(), t.TempDir(), srv.URL, "0.5.0")
}

// TestSummary_Negative404Cached: a 404 (unknown appid) is a stable answer,
// so it is cached like a success within the same TTL: the second call within
// the TTL returns ErrNotFound without touching the server.
func TestSummary_Negative404Cached(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>404 Not Found</body></html>`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	for i := 0; i < 2; i++ {
		_, live, err := c.Summary(context.Background(), "9999999")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("call %d err = %v, want ErrNotFound", i+1, err)
		}
		if want := i == 0; live != want {
			t.Errorf("call %d live = %v, want %v (cached 404 must not re-request)", i+1, live, want)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits = %d, want 1 (the negative answer is cached)", got)
	}
	t.Logf("two calls, %d server hit; second 404 served from cache", hits.Load())
}

// TestSummary_RejectsHugeBody: the response body is capped (1 MiB) before
// decoding, so a hostile or broken endpoint streaming megabytes surfaces as
// a decode error instead of an unbounded read.
func TestSummary_RejectsHugeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"tier":"gold","pad":"`)
		for i := 0; i < 5<<20; i++ {
			_, _ = w.Write([]byte{'x'})
		}
		_, _ = fmt.Fprint(w, `"}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _, err := c.Summary(context.Background(), "1245620")
	if err == nil {
		t.Fatal("expected an error on a >1MiB body, got nil (body cap missing?)")
	}
	t.Logf("huge body rejected: %v", err)
}

func TestSummary_ParsesTierConfidence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/reports/summaries/1245620.json" {
			t.Errorf("request path = %q, want /api/v1/reports/summaries/1245620.json", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, testSummaryJSON)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	sum, live, err := c.Summary(context.Background(), "1245620")
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if !live {
		t.Error("live = false on fresh fetch, want true (no cache entry existed)")
	}
	if sum.Tier != "gold" || sum.Confidence != "strong" {
		t.Errorf("got tier=%q confidence=%q, want gold/strong", sum.Tier, sum.Confidence)
	}
	if sum.Score != 82 || sum.Total != 1371 {
		t.Errorf("got score=%d total=%d, want 82/1371", sum.Score, sum.Total)
	}
	if sum.BestReportedTier != "platinum" || sum.TrendingTier != "gold" {
		t.Errorf("got bestReported=%q trending=%q, want platinum/gold", sum.BestReportedTier, sum.TrendingTier)
	}
	t.Logf("appid=1245620 tier=%s confidence=%s score=%d total=%d", sum.Tier, sum.Confidence, sum.Score, sum.Total)
}

func TestSummary_404HTMLReturnsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `<!DOCTYPE html><html><body>404 Not Found</body></html>`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _, err := c.Summary(context.Background(), "9999999")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound (unknown appid answers 404 with an HTML body, not JSON)", err)
	}
	t.Logf("404 HTML body: %v", err)
}

func TestSummary_InvalidJSONErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{not json`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _, err := c.Summary(context.Background(), "1245620")
	if err == nil {
		t.Fatal("expected a decode error on invalid JSON, got nil")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v must NOT be ErrNotFound (status was 200; the body was simply corrupt)", err)
	}
	t.Logf("invalid JSON: %v", err)
}

func TestSummary_CooldownAfter429(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _, err := c.Summary(context.Background(), "1245620")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("first call err = %v, want ErrRateLimited on HTTP 429 without cache", err)
	}
	_, _, err = c.Summary(context.Background(), "1245620")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second call err = %v, want ErrRateLimited inside cooldown", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits = %d, want 1 (cooldown must not touch the network)", got)
	}
	t.Logf("429 -> cooldown: %d server hit across two calls", hits.Load())
}

func TestSummary_UsesCacheWithinTTL(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, testSummaryJSON)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, live, err := c.Summary(context.Background(), "1245620"); err != nil || !live {
		t.Fatalf("first call: live=%v err=%v, want true/nil", live, err)
	}
	sum, live, err := c.Summary(context.Background(), "1245620")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if live {
		t.Error("second call live = true, want false (7d TTL cache hit)")
	}
	if sum.Tier != "gold" {
		t.Errorf("cached tier = %q, want gold", sum.Tier)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits = %d, want 1", got)
	}
	t.Logf("two calls, %d server hit; second served from cache", hits.Load())
}

func TestSummary_RefetchesAfterTTL(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, testSummaryJSON)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	now := time.Now()
	c.now = func() time.Time { return now }

	if _, _, err := c.Summary(context.Background(), "1245620"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	now = now.Add(cacheTTL + time.Hour)
	if _, live, err := c.Summary(context.Background(), "1245620"); err != nil || !live {
		t.Fatalf("second call after TTL: live=%v err=%v, want true/nil", live, err)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("server hits = %d, want 2 (expired cache must refetch)", got)
	}
	t.Logf("cache expired after TTL: %d server hits", hits.Load())
}
