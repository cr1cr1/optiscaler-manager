// Package wikidata resolves game cover art via Wikidata and Wikimedia
// Commons, the keyless last-resort art source for games with no Steam or
// PCGamingWiki presence (console titles installed via emulator). Entity
// search (wbsearchentities) is scored with the shared gid matcher — the
// best-accepted entity's P18 image is the cover, served through Commons'
// Special:FilePath. The client mirrors the pcgw package's discipline:
// request pacing, a short in-memory cooldown after 429/5xx, and a 30d
// disk cache with negatives (issue 030).
package wikidata

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

	"github.com/cr1cr1/optiscaler-manager/internal/gid"
)

// ErrNoMatch is returned when no acceptable entity (or no P18 image on
// it) exists for the title.
var ErrNoMatch = errors.New("wikidata: no entity match")

// ErrRateLimited is returned on HTTP 429/5xx with no cached answer.
var ErrRateLimited = errors.New("wikidata: rate limited")

const (
	cooldown        = 5 * time.Minute
	cacheTTL        = 30 * 24 * time.Hour
	minSpacing      = 2 * time.Second
	maxBodyBytes    = 1 << 20
	defaultAPIBase  = "https://www.wikidata.org"
	defaultFileBase = "https://commons.wikimedia.org"
	searchResultMax = 8
)

// Client queries the Wikidata API and builds Commons file URLs.
type Client struct {
	http      *http.Client
	cacheDir  string
	apiBase   string
	fileBase  string
	userAgent string
	now       func() time.Time

	mu            sync.Mutex
	lastReq       time.Time
	cooldownUntil time.Time
}

// New returns a Client. A descriptive User-Agent is mandatory per the
// Wikimedia API policy.
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
		apiBase:   defaultAPIBase,
		fileBase:  defaultFileBase,
		userAgent: "optiscaler-manager/" + version + " (https://github.com/cr1cr1/optiscaler-manager)",
		now:       time.Now,
	}
}

// NewWithBases is New with explicit API and file hosts, for tests.
func NewWithBases(httpClient *http.Client, cacheDir, apiBase, fileBase, version string) *Client {
	c := New(httpClient, cacheDir, version)
	c.apiBase = apiBase
	c.fileBase = fileBase
	return c
}

// SearchCoverFile resolves a game title to its Commons cover filename:
// the best-scored acceptable entity's first P18 image. Answers come from
// the disk cache when fresh; misses are cached as negatives.
func (c *Client) SearchCoverFile(ctx context.Context, title string) (fileName string, live bool, err error) {
	title = strings.TrimSpace(title)
	key := gid.Normalize(title)
	if key == "" {
		return "", false, errors.New("wikidata: empty title")
	}
	if hit, ok := c.readCache(key); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return "", false, fmt.Errorf("%w for %q (cached)", ErrNoMatch, title)
		}
		return hit.Value, false, nil
	}

	var search struct {
		Search []struct {
			ID      string   `json:"id"`
			Label   string   `json:"label"`
			Aliases []string `json:"aliases"`
		} `json:"search"`
	}
	q := "/w/api.php?action=wbsearchentities&type=item&language=en" +
		"&limit=" + fmt.Sprintf("%d", searchResultMax) +
		"&search=" + url.QueryEscape(title) + "&format=json"
	if err := c.get(ctx, q, &search); err != nil {
		return "", true, err
	}
	// Every label and alias is scored; the best acceptable entity wins —
	// the first hit is not always the right one.
	bestID, bestScore := "", -1
	for _, e := range search.Search {
		for _, name := range append([]string{e.Label}, e.Aliases...) {
			if score := gid.Score(title, name, false); score > bestScore {
				bestID, bestScore = e.ID, score
			}
		}
	}
	if bestID == "" || !gid.Accept(bestScore, false) {
		c.writeCache(key, cachedValue{FetchedAt: c.now(), NoMatch: true})
		return "", true, fmt.Errorf("%w for %q (no acceptable entity)", ErrNoMatch, title)
	}

	var entities struct {
		Entities map[string]struct {
			Claims map[string][]struct {
				Mainsnak struct {
					Datavalue struct {
						Value string `json:"value"`
					} `json:"datavalue"`
				} `json:"mainsnak"`
			} `json:"claims"`
		} `json:"entities"`
	}
	q = "/w/api.php?action=wbgetentities&props=claims&format=json&ids=" + url.QueryEscape(bestID)
	if err := c.get(ctx, q, &entities); err != nil {
		return "", true, err
	}
	for _, snak := range entities.Entities[bestID].Claims["P18"] {
		if v := strings.TrimSpace(snak.Mainsnak.Datavalue.Value); v != "" {
			fileName = v
			break
		}
	}
	if fileName == "" {
		c.writeCache(key, cachedValue{FetchedAt: c.now(), NoMatch: true})
		return "", true, fmt.Errorf("%w for %q (entity %s has no P18 image)", ErrNoMatch, title, bestID)
	}
	c.writeCache(key, cachedValue{Value: fileName, FetchedAt: c.now()})
	return fileName, true, nil
}

// FileURL builds the Commons Special:FilePath URL for a file, sized to
// width via the thumbnail redirect.
func (c *Client) FileURL(fileName string, width int) string {
	name := strings.ReplaceAll(strings.TrimSpace(fileName), " ", "_")
	return c.fileBase + "/wiki/Special:FilePath/" + url.PathEscape(name) +
		"?width=" + fmt.Sprintf("%d", width)
}

// cachedValue is one disk-cached answer; NoMatch marks a negative.
type cachedValue struct {
	Value     string    `json:"value,omitempty"`
	NoMatch   bool      `json:"no_match,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}

func (c *Client) cachePath(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.cacheDir, "cover_"+hex.EncodeToString(sum[:])[:16]+".json")
}

func (c *Client) readCache(key string) (cachedValue, bool) {
	f, err := os.Open(c.cachePath(key))
	if err != nil {
		return cachedValue{}, false
	}
	defer func() { _ = f.Close() }()
	var v cachedValue
	if err := json.NewDecoder(io.LimitReader(f, 1<<16)).Decode(&v); err != nil {
		return cachedValue{}, false
	}
	return v, true
}

func (c *Client) writeCache(key string, v cachedValue) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(c.cacheDir, ".wd-*")
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+query, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wikidata: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		c.mu.Lock()
		c.cooldownUntil = c.now().Add(cooldown)
		c.mu.Unlock()
		return fmt.Errorf("%w (HTTP %d)", ErrRateLimited, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wikidata: unexpected HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(payload)
}
