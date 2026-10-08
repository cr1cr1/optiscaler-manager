// Package gogdb resolves game titles and cover art against GOG's keyless
// Galaxy catalog API (catalog.gog.com/v1/catalog, query=like:<term>):
// product titles for identification, coverVertical portrait art for
// covers. The like: ranking degrades with junk tokens exactly like
// Steam's storesearch, so callers walk the same query variants and score
// with the shared gid matcher. The client mirrors the pcgw/wikidata
// discipline: request pacing, an in-memory cooldown after 429/5xx, and a
// 30d disk cache with negatives (issue 034).
package gogdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrNoMatch is returned when the catalog has no game products for the
// query.
var ErrNoMatch = errors.New("gogdb: no catalog match")

// ErrRateLimited is returned on HTTP 429/5xx (and during the cooldown
// that follows) with no cached answer.
var ErrRateLimited = errors.New("gogdb: rate limited")

const (
	cooldown = 5 * time.Minute
	cacheTTL = 30 * 24 * time.Hour
	// The catalog is a CDN-backed read API: pcgw's 2s spacing is the
	// wiki's published limit, not GOG's. Multi-variant walks stay fast.
	minSpacing     = 500 * time.Millisecond
	maxBodyBytes   = 4 << 20 // catalog payloads carry screenshot arrays
	searchLimit    = 24
	defaultBaseURL = "https://catalog.gog.com"
)

// Product is one game entry of the catalog: id/title for matching,
// coverVertical for art (empty when the product has none).
type Product struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	CoverVertical string `json:"cover_vertical"`
}

// Client queries the GOG catalog API.
type Client struct {
	http      *http.Client
	cacheDir  string
	baseURL   string
	userAgent string
	now       func() time.Time

	mu            sync.Mutex
	lastReq       time.Time
	cooldownUntil time.Time
}

// New returns a Client. A descriptive User-Agent is mandatory: CD
// Projekt's edge rejects requests without one.
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
		userAgent: "optiscaler-manager/" + version + " (https://github.com/cr1cr1/optiscaler-manager)",
		now:       time.Now,
	}
}

// NewWithBaseURL is New with an explicit API host, for tests.
func NewWithBaseURL(httpClient *http.Client, cacheDir, baseURL, version string) *Client {
	c := New(httpClient, cacheDir, version)
	c.baseURL = baseURL
	return c
}

// SearchProducts resolves a free-text term to the catalog's game
// products (relevance-ranked as GOG ranks them — callers score and
// pick). DLC and movies are filtered out. Answers come from the disk
// cache when fresh; empty results are cached as negatives.
func (c *Client) SearchProducts(ctx context.Context, term string) (products []Product, live bool, err error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, false, errors.New("gogdb: empty term")
	}
	// Key by the raw term (case-folded), never a normalized form: callers
	// walk query variants (raw → normalized → truncated) and a raw junky
	// query's negative must not poison the normalized variant's lookup.
	key := strings.ToLower(term)
	if hit, ok := c.readCache(key); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return nil, false, fmt.Errorf("%w for %q (cached)", ErrNoMatch, term)
		}
		return hit.Products, false, nil
	}
	q := "/v1/catalog?countryCode=US&currency=USD&locale=en-US" +
		"&limit=" + fmt.Sprintf("%d", searchLimit) +
		"&query=like:" + url.QueryEscape(term)
	var payload struct {
		Products []struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			CoverVertical string `json:"coverVertical"`
			ProductType   string `json:"productType"`
		} `json:"products"`
	}
	if err := c.get(ctx, q, &payload); err != nil {
		return nil, true, err
	}
	for _, p := range payload.Products {
		if p.ProductType != "game" || p.Title == "" || p.ID == "" {
			continue
		}
		products = append(products, Product{ID: p.ID, Title: p.Title, CoverVertical: p.CoverVertical})
	}
	if len(products) == 0 {
		c.writeCache(key, cachedSearch{FetchedAt: c.now(), NoMatch: true})
		return nil, true, fmt.Errorf("%w for %q (empty results)", ErrNoMatch, term)
	}
	c.writeCache(key, cachedSearch{Products: products, FetchedAt: c.now()})
	return products, true, nil
}

// cachedSearch is one disk-cached answer; NoMatch marks a negative.
type cachedSearch struct {
	Products  []Product `json:"products,omitempty"`
	NoMatch   bool      `json:"no_match,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}

func (c *Client) cachePath(key string) string {
	sum := sha256.Sum256([]byte("search:" + key))
	return filepath.Join(c.cacheDir, "search_"+hex.EncodeToString(sum[:])[:16]+".json")
}

func (c *Client) readCache(key string) (cachedSearch, bool) {
	f, err := os.Open(c.cachePath(key))
	if err != nil {
		return cachedSearch{}, false
	}
	defer func() { _ = f.Close() }()
	var v cachedSearch
	if err := json.NewDecoder(io.LimitReader(f, 1<<20)).Decode(&v); err != nil {
		return cachedSearch{}, false
	}
	return v, true
}

func (c *Client) writeCache(key string, v cachedSearch) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(c.cacheDir, ".gog-*")
	if err != nil {
		return
	}
	if err := json.NewEncoder(tmp).Encode(v); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	_ = os.Rename(tmp.Name(), c.cachePath(key))
}

// get performs one paced API request with in-memory cooldown-on-429/5xx
// handling and decodes the (bounded) JSON body.
func (c *Client) get(ctx context.Context, query string, payload any) error {
	c.mu.Lock()
	if c.now().Before(c.cooldownUntil) {
		c.mu.Unlock()
		return ErrRateLimited
	}
	if d := c.lastReq.Add(minSpacing).Sub(c.now()); d > 0 {
		select {
		case <-ctx.Done():
			c.mu.Unlock()
			return ctx.Err()
		case <-time.After(d):
		}
	}
	c.lastReq = c.now()
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+query, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gogdb: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		c.mu.Lock()
		c.cooldownUntil = c.now().Add(cooldown)
		c.mu.Unlock()
		return fmt.Errorf("%w (HTTP %d)", ErrRateLimited, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gogdb: unexpected HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(payload)
}
