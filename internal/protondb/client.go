// Package protondb fetches ProtonDB report summaries for Steam app IDs via
// the public summaries endpoint (no auth), with a per-appid disk cache,
// polite client-side request pacing, and a short cooldown after
// rate-limit/server-error responses.
//
// Unknown app IDs answer HTTP 404 with an HTML error page (not JSON), so
// the status code is guarded first and the content type second: a 404 is
// ErrNotFound, never a decode error.
package protondb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned when ProtonDB has no reports for the appid
// (HTTP 404, served as an HTML page by the endpoint).
var ErrNotFound = errors.New("protondb: no reports for appid")

// ErrRateLimited is returned when the endpoint rate-limits or server-errors
// (HTTP 429/5xx) and no cached summary is available to serve from.
var ErrRateLimited = errors.New("protondb: rate limited")

const (
	// cooldown is the client-side pause after a 429/5xx response.
	cooldown = 5 * time.Minute

	// cacheTTL is how long a summary is trusted before refetching.
	cacheTTL = 7 * 24 * time.Hour

	// minSpacing is the minimum gap between live requests (politeness;
	// the endpoint publishes no rate limit).
	minSpacing = 250 * time.Millisecond

	defaultBaseURL = "https://www.protondb.com"
	summariesPath  = "/api/v1/reports/summaries/"

	cooldownFile = "cooldown.json"
)

// Summary mirrors the ProtonDB summary JSON fields actually used.
type Summary struct {
	Tier             string `json:"tier"`
	Confidence       string `json:"confidence"`
	Score            int    `json:"score"`
	Total            int    `json:"total"`
	BestReportedTier string `json:"bestReportedTier"`
	TrendingTier     string `json:"trendingTier"`
	ProvisionalTier  string `json:"provisionalTier,omitempty"`
}

// Client fetches ProtonDB report summaries.
type Client struct {
	http      *http.Client
	cacheDir  string
	baseURL   string // unexported so tests can point at httptest servers
	userAgent string

	// now is a clock hook for tests (cache TTL, cooldown expiry, pacing).
	now func() time.Time

	mu          sync.Mutex
	lastRequest time.Time // last live request time, for minSpacing pacing
}

// New returns a Client. A nil httpClient uses http.DefaultClient. version
// becomes the User-Agent suffix ("optiscaler-manager/<version>"); an empty
// version reports "dev".
func New(httpClient *http.Client, cacheDir, version string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if version == "" {
		version = "dev"
	}
	return &Client{
		http:      httpClient,
		cacheDir:  cacheDir,
		baseURL:   defaultBaseURL,
		userAgent: "optiscaler-manager/" + version,
		now:       time.Now,
	}
}

// NewWithBaseURL is New with an explicit API base URL, for tests and
// OM_PROTONDB_BASE_URL-style overrides.
func NewWithBaseURL(httpClient *http.Client, cacheDir, baseURL, version string) *Client {
	c := New(httpClient, cacheDir, version)
	c.baseURL = baseURL
	return c
}

// Summary returns the report summary for appid. fromCache reports that the
// summary came from the disk cache (fresh within the 7d TTL, or stale as a
// last resort inside the post-429/5xx cooldown).
func (c *Client) Summary(ctx context.Context, appid string) (sum Summary, fromCache bool, err error) {
	if strings.TrimSpace(appid) == "" {
		return Summary{}, false, errors.New("protondb: empty appid")
	}
	if hit, ok := c.readCache(appid); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		return hit.Summary, true, nil
	}
	if c.inCooldown() {
		if hit, ok := c.readCache(appid); ok {
			return hit.Summary, true, nil
		}
		return Summary{}, false, fmt.Errorf("%w (cooldown active, no cached summary)", ErrRateLimited)
	}
	sum, err = c.fetch(ctx, appid)
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			if hit, ok := c.readCache(appid); ok {
				return hit.Summary, true, nil
			}
		}
		return Summary{}, false, err
	}
	return sum, false, nil
}

// fetch performs the live request, pacing it, recording a cooldown on
// 429/5xx, guarding the 404/HTML unknown-appid case before decoding, and
// persisting a successful summary to the disk cache.
func (c *Client) fetch(ctx context.Context, appid string) (Summary, error) {
	c.pace(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+summariesPath+appid+".json", nil)
	if err != nil {
		return Summary{}, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Summary{}, fmt.Errorf("protondb: summary %s: %w", appid, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Status first: unknown appids answer 404 with an HTML body.
	if resp.StatusCode == http.StatusNotFound {
		return Summary{}, fmt.Errorf("%w %s (HTTP 404)", ErrNotFound, appid)
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		_ = c.writeCooldown(c.now())
		return Summary{}, fmt.Errorf("%w (HTTP %d)", ErrRateLimited, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return Summary{}, fmt.Errorf("protondb: summary %s: unexpected HTTP %d", appid, resp.StatusCode)
	}
	// Content type second: a 200 with a non-JSON body is an endpoint
	// surprise, not a parse error.
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(strings.ToLower(ct), "json") {
		return Summary{}, fmt.Errorf("protondb: summary %s: unexpected content type %q", appid, ct)
	}

	var sum Summary
	if err := json.NewDecoder(resp.Body).Decode(&sum); err != nil {
		return Summary{}, fmt.Errorf("protondb: decode summary %s: %w", appid, err)
	}
	if err := c.writeCache(appid, sum); err != nil {
		return Summary{}, err
	}
	return sum, nil
}

// pace blocks until minSpacing has elapsed since the last live request and
// stamps the request time. A cancelled context shortens the wait; the
// subsequent request fails on its own.
func (c *Client) pace(ctx context.Context) {
	c.mu.Lock()
	wait := minSpacing - c.now().Sub(c.lastRequest)
	c.mu.Unlock()
	if wait > 0 {
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
		case <-t.C:
		}
	}
	c.mu.Lock()
	c.lastRequest = c.now()
	c.mu.Unlock()
}
