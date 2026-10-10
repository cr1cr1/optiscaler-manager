// Package umudb resolves (store, codename) pairs to umu IDs via the
// umu-database at https://umu.openwinecomponents.org, so non-Steam
// games get their protonfixes (issue 064). The database only covers
// games that NEED fixes; a miss (ErrNoMatch) simply means the game
// runs out of the box. Lookups are cached on disk for 30 days (hits
// and misses alike); failures are plain errors so a launch degrades
// to "umu-default" instead of blocking.
package umudb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/jsoncache"
)

// ErrNoMatch marks "no entry for this (store, codename)" — cached like
// a hit so fix-free games cost no request per launch.
var ErrNoMatch = errors.New("umudb: no entry")

// cacheTTL mirrors the other online clients (steam, sgdb).
const cacheTTL = 30 * 24 * time.Hour

const defaultBaseURL = "https://umu.openwinecomponents.org/umu_api.php"

// Client queries the umu-database and caches results under cacheDir.
type Client struct {
	baseURL  string // test seam; production is defaultBaseURL
	http     *http.Client
	cacheDir string
	ua       string
	now      func() time.Time
}

// New builds the production client.
func New(httpClient *http.Client, cacheDir, userAgent string) *Client {
	return &Client{
		baseURL:  defaultBaseURL,
		http:     httpClient,
		cacheDir: cacheDir,
		ua:       userAgent,
		now:      time.Now,
	}
}

type cachedEntry struct {
	UmuID     string    `json:"umu_id,omitempty"`
	NoMatch   bool      `json:"no_match,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}

// Lookup returns the umu ID for a (store, codename) pair — e.g.
// ("egs", "Heather") → "umu-1174180". ErrNoMatch (cached) when the
// database has no entry.
func (c *Client) Lookup(ctx context.Context, store, codename string) (string, error) {
	key := cacheKey(store, codename)
	if hit, ok := jsoncache.Read[cachedEntry](filepath.Join(c.cacheDir, key+".json")); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return "", ErrNoMatch
		}
		return hit.UmuID, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"?store="+url.QueryEscape(store)+"&codename="+url.QueryEscape(codename), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.ua)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err // transport failure: retriable, never cached
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("umudb: %s", resp.Status)
	}
	var rows []struct {
		Title string `json:"title"`
		UmuID string `json:"umu_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return "", fmt.Errorf("umudb: decode: %w", err)
	}
	if len(rows) == 0 || rows[0].UmuID == "" {
		_ = jsoncache.Write(filepath.Join(c.cacheDir, key+".json"), cachedEntry{NoMatch: true, FetchedAt: c.now()})
		return "", ErrNoMatch
	}
	_ = jsoncache.Write(filepath.Join(c.cacheDir, key+".json"), cachedEntry{UmuID: rows[0].UmuID, FetchedAt: c.now()})
	return rows[0].UmuID, nil
}

// cacheKey makes a filename-safe key for a (store, codename) pair.
func cacheKey(store, codename string) string {
	var b strings.Builder
	for _, r := range store + ":" + codename {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
