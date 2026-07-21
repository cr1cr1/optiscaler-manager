package steam

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
	"time"
)

// Store endpoints (store.steampowered.com, no auth): appdetails resolves a
// Steam app id to its canonical store name and developer, storesearch
// resolves a free-text title to candidate store items with platform flags.
// Both share the client's request pacing, post-429/5xx cooldown, and 30d
// disk-cache policy (negatives cached like successes).

const defaultStoreURL = "https://store.steampowered.com"

// NewWithBaseURLs is NewWithBaseURL with an explicit store API host too,
// for tests and overrides.
func NewWithBaseURLs(httpClient *http.Client, cacheDir, baseURL, storeURL, version string) *Client {
	c := NewWithBaseURL(httpClient, cacheDir, baseURL, version)
	c.storeURL = storeURL
	return c
}

// StoreItem is one app row from the storesearch endpoint.
type StoreItem struct {
	ID      string
	Name    string
	Windows bool
	Mac     bool
	Linux   bool
}

// AppDetails resolves an appid to its canonical store name and developer.
// Answers come from the disk cache when fresh; a success:false answer is
// cached as a negative (ErrNoMatch).
func (c *Client) AppDetails(ctx context.Context, appid string) (name, developer string, live bool, err error) {
	appid = strings.TrimSpace(appid)
	if appid == "" {
		return "", "", false, errors.New("steam: empty appid")
	}
	if hit, ok := c.readAppDetailsCache(appid); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return "", "", false, fmt.Errorf("%w for appid %s (cached)", ErrNoMatch, appid)
		}
		return hit.Name, hit.Developer, false, nil
	}
	body, err := c.get(ctx, c.storeURL+"/api/appdetails?appids="+url.QueryEscape(appid))
	if err != nil {
		return "", "", true, err
	}
	var payload map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Name       string   `json:"name"`
			Developers []string `json:"developers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", true, fmt.Errorf("steam: decode appdetails for %s: %w", appid, err)
	}
	entry, ok := payload[appid]
	if !ok || !entry.Success || entry.Data.Name == "" {
		c.writeAppDetailsCache(appid, cachedAppDetails{FetchedAt: c.now(), NoMatch: true})
		return "", "", true, fmt.Errorf("%w for appid %s", ErrNoMatch, appid)
	}
	dev := ""
	if len(entry.Data.Developers) > 0 {
		dev = entry.Data.Developers[0]
	}
	c.writeAppDetailsCache(appid, cachedAppDetails{Name: entry.Data.Name, Developer: dev, FetchedAt: c.now()})
	return entry.Data.Name, dev, true, nil
}

// StoreSearch searches the store for term and returns the candidate items
// (relevance-ordered). An empty result is cached as a negative
// (ErrNoMatch).
func (c *Client) StoreSearch(ctx context.Context, term string) (items []StoreItem, live bool, err error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, false, errors.New("steam: empty search term")
	}
	if hit, ok := c.readStoreSearchCache(term); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return nil, false, fmt.Errorf("%w for %q (cached)", ErrNoMatch, term)
		}
		return hit.Items, false, nil
	}
	body, err := c.get(ctx, c.storeURL+"/api/storesearch/?term="+url.QueryEscape(term)+"&cc=us&l=en")
	if err != nil {
		return nil, true, err
	}
	var payload struct {
		Items []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			ID        int64  `json:"id"`
			Platforms struct {
				Windows bool `json:"windows"`
				Mac     bool `json:"mac"`
				Linux   bool `json:"linux"`
			} `json:"platforms"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, true, fmt.Errorf("steam: decode storesearch for %q: %w", term, err)
	}
	for _, it := range payload.Items {
		if it.Type != "app" || it.Name == "" || it.ID == 0 {
			continue
		}
		items = append(items, StoreItem{
			ID:      fmt.Sprintf("%d", it.ID),
			Name:    it.Name,
			Windows: it.Platforms.Windows,
			Mac:     it.Platforms.Mac,
			Linux:   it.Platforms.Linux,
		})
	}
	if len(items) == 0 {
		c.writeStoreSearchCache(term, cachedStoreSearch{FetchedAt: c.now(), NoMatch: true})
		return nil, true, fmt.Errorf("%w for %q (empty results)", ErrNoMatch, term)
	}
	c.writeStoreSearchCache(term, cachedStoreSearch{Items: items, FetchedAt: c.now()})
	return items, true, nil
}

// get performs one paced store request with cooldown-on-429/5xx handling
// and returns the (bounded) body.
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	if c.inCooldown() {
		return nil, fmt.Errorf("%w (cooldown active)", ErrRateLimited)
	}
	c.pace(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("steam: store request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		_ = c.writeCooldown(c.now())
		return nil, fmt.Errorf("%w (HTTP %d)", ErrRateLimited, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam: store request: unexpected HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
}

// cachedAppDetails is the persisted appid → store-name mapping.
type cachedAppDetails struct {
	Name      string    `json:"name"`
	Developer string    `json:"developer,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	NoMatch   bool      `json:"no_match,omitempty"`
}

// cachedStoreSearch is the persisted term → items result page.
type cachedStoreSearch struct {
	Items     []StoreItem `json:"items"`
	FetchedAt time.Time   `json:"fetched_at"`
	NoMatch   bool        `json:"no_match,omitempty"`
}

func (c *Client) readAppDetailsCache(appid string) (cachedAppDetails, bool) {
	var cs cachedAppDetails
	data, err := os.ReadFile(filepath.Join(c.cacheDir, "appdetails_"+appid+".json"))
	if err != nil {
		return cs, false
	}
	if err := json.Unmarshal(data, &cs); err != nil {
		return cachedAppDetails{}, false
	}
	return cs, true
}

func (c *Client) writeAppDetailsCache(appid string, cs cachedAppDetails) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(cs)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(c.cacheDir, "appdetails_"+appid+".json"), data, 0o644)
}

func storeSearchFile(cacheDir, term string) string {
	sum := sha256.Sum256([]byte(normalize(term)))
	return filepath.Join(cacheDir, "storesearch_"+hex.EncodeToString(sum[:])[:16]+".json")
}

func (c *Client) readStoreSearchCache(term string) (cachedStoreSearch, bool) {
	var cs cachedStoreSearch
	data, err := os.ReadFile(storeSearchFile(c.cacheDir, term))
	if err != nil {
		return cs, false
	}
	if err := json.Unmarshal(data, &cs); err != nil {
		return cachedStoreSearch{}, false
	}
	return cs, true
}

func (c *Client) writeStoreSearchCache(term string, cs cachedStoreSearch) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(cs)
	if err != nil {
		return
	}
	_ = os.WriteFile(storeSearchFile(c.cacheDir, term), data, 0o644)
}
