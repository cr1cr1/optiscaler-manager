// Package pcgw resolves game titles against PCGamingWiki, the secondary
// canonical source for games Steam does not carry (GOG/off-store
// installs). Keyless MediaWiki API: title search via opensearch, infobox
// cover filenames via the page wikitext (prop=revisions), and thumbnails
// via prop=imageinfo. The Cargo API is intentionally NOT used: the wiki
// now rejects anonymous Cargo queries with HTTP 200 + an error envelope —
// such envelopes are live errors here, never cached as negatives (issue
// 024). The client mirrors the steam package's discipline: 30 req/min
// pacing (the wiki's published limit), a short cooldown after 429/5xx,
// and a 30d disk cache with negatives.
package pcgw

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
	"regexp"
	"strings"
	"sync"
	"time"
)

// ErrNoMatch is returned when the wiki has no page for the query.
var ErrNoMatch = errors.New("pcgw: no page match")

// ErrRateLimited is returned on HTTP 429/5xx with no cached answer.
var ErrRateLimited = errors.New("pcgw: rate limited")

// ErrAPI is returned when the wiki answers HTTP 200 with an error
// envelope (e.g. permissiondenied on a restricted module). It is a live
// failure: callers must not cache it as a negative.
var ErrAPI = errors.New("pcgw: API error")

const (
	cooldown       = 5 * time.Minute
	cacheTTL       = 30 * 24 * time.Hour
	minSpacing     = 2 * time.Second // wiki publishes 30 req/min
	maxBodyBytes   = 1 << 20
	defaultBaseURL = "https://www.pcgamingwiki.com"
	cooldownFile   = "cooldown.json"
)

// Client queries the PCGamingWiki API.
type Client struct {
	http      *http.Client
	cacheDir  string
	baseURL   string
	userAgent string
	now       func() time.Time
	mu        sync.Mutex
	lastReq   time.Time
}

// New returns a Client. A descriptive User-Agent is mandatory per the
// wiki's API policy.
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

// SearchTitles resolves a free-text title to the wiki's candidate page
// titles (all opensearch hits, ranked as the wiki ranks them — callers
// score and pick, since the first hit is not always the right one).
// Answers come from the disk cache when fresh; empty results are cached
// as negatives.
func (c *Client) SearchTitles(ctx context.Context, term string) (titles []string, live bool, err error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, false, errors.New("pcgw: empty term")
	}
	if hit, ok := c.readTitlesCache(term); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return nil, false, fmt.Errorf("%w for %q (cached)", ErrNoMatch, term)
		}
		return hit.Values, false, nil
	}
	var payload []interface{}
	if err := c.get(ctx, "/w/api.php?action=opensearch&search="+url.QueryEscape(term)+"&redirects=resolve&format=json", &payload); err != nil {
		return nil, true, err
	}
	if len(payload) >= 2 {
		if names, ok := payload[1].([]interface{}); ok {
			for _, n := range names {
				if s, ok := n.(string); ok && s != "" {
					titles = append(titles, s)
				}
			}
		}
	}
	if len(titles) == 0 {
		c.writeTitlesCache(term, cachedTitles{FetchedAt: c.now(), NoMatch: true})
		return nil, true, fmt.Errorf("%w for %q (empty results)", ErrNoMatch, term)
	}
	c.writeTitlesCache(term, cachedTitles{Values: titles, FetchedAt: c.now()})
	return titles, true, nil
}

// get performs one paced, cached-cooldown-aware JSON GET. A MediaWiki
// error envelope (HTTP 200 + {"error":{...}}) is an ErrAPI live failure,
// never a decodable result.
func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	if c.inCooldown() {
		return fmt.Errorf("%w (cooldown active)", ErrRateLimited)
	}
	c.pace(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("pcgw: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		_ = c.writeCooldown(c.now())
		return fmt.Errorf("%w (HTTP %d)", ErrRateLimited, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pcgw: unexpected HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("pcgw: read body: %w", err)
	}
	var env struct {
		Error *struct {
			Code string `json:"code"`
			Info string `json:"info"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err == nil && env.Error != nil {
		return fmt.Errorf("%w %s: %s", ErrAPI, env.Error.Code, env.Error.Info)
	}
	return json.Unmarshal(body, out)
}

// pace blocks until minSpacing has elapsed since the last live request.
func (c *Client) pace(ctx context.Context) {
	c.mu.Lock()
	wait := minSpacing - c.now().Sub(c.lastReq)
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
	c.lastReq = c.now()
	c.mu.Unlock()
}

type cooldownState struct {
	// LastLimited persists under the historical key "last_attempt" —
	// the on-disk format stays (429/5xx responses only).
	LastLimited time.Time `json:"last_attempt"`
}

func (c *Client) inCooldown() bool {
	data, err := os.ReadFile(filepath.Join(c.cacheDir, cooldownFile))
	if err != nil {
		return false
	}
	var state cooldownState
	if err := json.Unmarshal(data, &state); err != nil {
		return false
	}
	return c.now().Sub(state.LastLimited) < cooldown
}

func (c *Client) writeCooldown(t time.Time) error {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cooldownState{LastLimited: t})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.cacheDir, cooldownFile), data, 0o644)
}

// cachedValue is one persisted lookup result (cover filename or thumbnail
// URL text, or a negative).
type cachedValue struct {
	Value     string    `json:"value,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	NoMatch   bool      `json:"no_match,omitempty"`
}

// cachedTitles is one persisted title-search result list (or a negative).
type cachedTitles struct {
	Values    []string  `json:"values,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	NoMatch   bool      `json:"no_match,omitempty"`
}

func (c *Client) cacheFile(kind, key string) string {
	// The "v2:" prefix orphans every pre-issue-024 entry: the dead-Cargo
	// era poisoned the cache with 30-day negatives that were really API
	// permission errors, and those files must never be read again.
	sum := sha256.Sum256([]byte("v2:" + kind + ":" + strings.ToLower(strings.TrimSpace(key))))
	return filepath.Join(c.cacheDir, kind+"_"+hex.EncodeToString(sum[:])[:16]+".json")
}

func (c *Client) readCache(kind, key string) (cachedValue, bool) {
	var cv cachedValue
	data, err := os.ReadFile(c.cacheFile(kind, key))
	if err != nil {
		return cv, false
	}
	if err := json.Unmarshal(data, &cv); err != nil {
		return cachedValue{}, false
	}
	return cv, true
}

func (c *Client) writeCache(kind, key string, cv cachedValue) {
	writeJSON(c.cacheDir, c.cacheFile(kind, key), cv)
}

func (c *Client) readTitlesCache(key string) (cachedTitles, bool) {
	var ct cachedTitles
	data, err := os.ReadFile(c.cacheFile("title", key))
	if err != nil {
		return ct, false
	}
	if err := json.Unmarshal(data, &ct); err != nil {
		return cachedTitles{}, false
	}
	return ct, true
}

func (c *Client) writeTitlesCache(key string, ct cachedTitles) {
	writeJSON(c.cacheDir, c.cacheFile("title", key), ct)
}

func writeJSON(dir, file string, v interface{}) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = os.WriteFile(file, data, 0o644)
}

// coverLineRe extracts the infobox `|cover = X` field from page wikitext
// (case-insensitive key). Only horizontal whitespace surrounds the value:
// `\s` would let an empty `|cover = ` line swallow the NEXT infobox line.
var coverLineRe = regexp.MustCompile(`(?im)^[ \t]*\|[ \t]*cover[ \t]*=[ \t]*(\S.*?)[ \t]*$`)

// CoverFile resolves a wiki page to its infobox cover image filename,
// parsed from the page wikitext (prop=revisions). Empty when the page is
// missing or its infobox has no cover — a true negative that IS cached.
func (c *Client) CoverFile(ctx context.Context, pageTitle string) (fileName string, live bool, err error) {
	pageTitle = strings.TrimSpace(pageTitle)
	if pageTitle == "" {
		return "", false, errors.New("pcgw: empty page title")
	}
	key := "cover:" + pageTitle
	if hit, ok := c.readCache("cover", key); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return "", false, fmt.Errorf("%w for %q (cached)", ErrNoMatch, pageTitle)
		}
		return hit.Value, false, nil
	}
	q := "/w/api.php?action=query&prop=revisions&rvprop=content&rvslots=main" +
		"&titles=" + url.QueryEscape(pageTitle) +
		"&format=json&formatversion=2"
	var payload struct {
		Query struct {
			Pages []struct {
				Revisions []struct {
					Slots struct {
						Main struct {
							Content string `json:"content"`
						} `json:"main"`
					} `json:"slots"`
				} `json:"revisions"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := c.get(ctx, q, &payload); err != nil {
		return "", true, err
	}
	for _, page := range payload.Query.Pages {
		for _, rev := range page.Revisions {
			if m := coverLineRe.FindStringSubmatch(rev.Slots.Main.Content); m != nil {
				fileName = m[1]
			}
		}
	}
	if fileName == "" {
		c.writeCache("cover", key, cachedValue{FetchedAt: c.now(), NoMatch: true})
		return "", true, fmt.Errorf("%w for %q", ErrNoMatch, pageTitle)
	}
	c.writeCache("cover", key, cachedValue{Value: fileName, FetchedAt: c.now()})
	return fileName, true, nil
}

// ImageThumbURL resolves a wiki file name to a sized thumbnail URL
// (imageinfo thumburl), used for the actual image download.
func (c *Client) ImageThumbURL(ctx context.Context, fileName string, width int) (thumbURL string, live bool, err error) {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "", false, errors.New("pcgw: empty file name")
	}
	key := fmt.Sprintf("thumb:%s:%d", fileName, width)
	if hit, ok := c.readCache("thumb", key); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return "", false, fmt.Errorf("%w for %q (cached)", ErrNoMatch, fileName)
		}
		return hit.Value, false, nil
	}
	q := "/w/api.php?action=query" +
		"&titles=" + url.QueryEscape("File:"+fileName) +
		"&prop=imageinfo&iiprop=url" +
		"&iiurlwidth=" + fmt.Sprintf("%d", width) +
		"&format=json"
	var payload struct {
		Query struct {
			Pages map[string]struct {
				ImageInfo []struct {
					ThumbURL string `json:"thumburl"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := c.get(ctx, q, &payload); err != nil {
		return "", true, err
	}
	for _, page := range payload.Query.Pages {
		if len(page.ImageInfo) > 0 && page.ImageInfo[0].ThumbURL != "" {
			thumbURL = page.ImageInfo[0].ThumbURL
			break
		}
	}
	if thumbURL == "" {
		c.writeCache("thumb", key, cachedValue{FetchedAt: c.now(), NoMatch: true})
		return "", true, fmt.Errorf("%w for %q", ErrNoMatch, fileName)
	}
	c.writeCache("thumb", key, cachedValue{Value: thumbURL, FetchedAt: c.now()})
	return thumbURL, true, nil
}
