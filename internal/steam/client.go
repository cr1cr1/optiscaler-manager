// Package steam resolves game titles to Steam app IDs via the public
// steamcommunity.com SearchApps endpoint (no auth), with a per-title disk
// cache, polite client-side request pacing, and a short cooldown after
// rate-limit/server-error responses.
//
// The endpoint answers a JSON array of candidate apps; only the first
// result is used, and only when its name is a plausible match for the
// query (case-insensitive equality or the query appearing in the name) —
// anything else is ErrNoMatch so callers never bind a title to an
// unrelated appid.
package steam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrNoMatch is returned when the search yields no result whose name is a
// plausible match for the query title.
var ErrNoMatch = errors.New("steam: no plausible search match")

// ErrRateLimited is returned when the endpoint rate-limits or server-errors
// (HTTP 429/5xx) and no cached result is available to serve from.
var ErrRateLimited = errors.New("steam: rate limited")

const (
	// cooldown is the client-side pause after a 429/5xx response.
	cooldown = 5 * time.Minute

	// cacheTTL is how long a resolved title → appid mapping is trusted.
	cacheTTL = 30 * 24 * time.Hour

	// minSpacing is the minimum gap between live requests (politeness;
	// the endpoint publishes no rate limit).
	minSpacing = 250 * time.Millisecond

	defaultBaseURL = "https://steamcommunity.com"
	searchPath     = "/actions/SearchApps/"

	cooldownFile = "cooldown.json"
)

// Client resolves titles to Steam app IDs.
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
// OM_STEAM_BASE_URL-style overrides.
func NewWithBaseURL(httpClient *http.Client, cacheDir, baseURL, version string) *Client {
	c := New(httpClient, cacheDir, version)
	c.baseURL = baseURL
	return c
}

// SearchApps resolves a game title to a Steam appid and the matched store
// name. Fresh cached mappings (30d TTL) are served without a network call;
// inside the post-429/5xx cooldown a stale cached mapping is served as a
// last resort.
func (c *Client) SearchApps(ctx context.Context, title string) (appid, name string, err error) {
	query := strings.TrimSpace(title)
	if query == "" {
		return "", "", errors.New("steam: empty title")
	}
	if hit, ok := c.readCache(query); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		return hit.AppID, hit.Name, nil
	}
	if c.inCooldown() {
		if hit, ok := c.readCache(query); ok {
			return hit.AppID, hit.Name, nil
		}
		return "", "", fmt.Errorf("%w (cooldown active, no cached result)", ErrRateLimited)
	}
	res, err := c.fetch(ctx, query)
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			if hit, ok := c.readCache(query); ok {
				return hit.AppID, hit.Name, nil
			}
		}
		return "", "", err
	}
	return res.AppID, res.Name, nil
}

// searchResult mirrors the SearchApps JSON fields actually used.
type searchResult struct {
	AppID string `json:"appid"`
	Name  string `json:"name"`
}

// fetch performs the live search, pacing the request, recording a cooldown
// on 429/5xx, and persisting a plausible first result to the disk cache.
func (c *Client) fetch(ctx context.Context, query string) (searchResult, error) {
	c.pace(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+searchPath+url.PathEscape(query), nil)
	if err != nil {
		return searchResult{}, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return searchResult{}, fmt.Errorf("steam: search %q: %w", query, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		_ = c.writeCooldown(c.now())
		return searchResult{}, fmt.Errorf("%w (HTTP %d)", ErrRateLimited, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return searchResult{}, fmt.Errorf("steam: search %q: unexpected HTTP %d", query, resp.StatusCode)
	}

	var results []searchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return searchResult{}, fmt.Errorf("steam: decode search results for %q: %w", query, err)
	}
	if len(results) == 0 {
		return searchResult{}, fmt.Errorf("%w for %q (empty results)", ErrNoMatch, query)
	}
	first := results[0]
	if !plausible(query, first.Name) {
		return searchResult{}, fmt.Errorf("%w for %q (first result %q implausible)", ErrNoMatch, query, first.Name)
	}
	if err := c.writeCache(query, first); err != nil {
		return searchResult{}, err
	}
	return first, nil
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

// plausible reports whether a result name is a believable match for the
// query: case-insensitive equality, or the query appearing anywhere in the
// name (covers prefix and substring).
func plausible(query, name string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	n := strings.ToLower(strings.TrimSpace(name))
	if q == "" || n == "" {
		return false
	}
	return n == q || strings.Contains(n, q)
}
