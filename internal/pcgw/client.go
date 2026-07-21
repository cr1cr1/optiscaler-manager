// Package pcgw resolves game titles against PCGamingWiki, the secondary
// canonical source for games Steam does not carry (GOG/off-store
// installs). Keyless MediaWiki API: title search via opensearch and
// Steam-appid reverse lookup via Cargo. The client mirrors the steam
// package's discipline: 30 req/min pacing (the wiki's published limit),
// a short cooldown after 429/5xx, and a 30d disk cache with negatives.
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
	"strings"
	"sync"
	"time"
)

// ErrNoMatch is returned when the wiki has no page for the query.
var ErrNoMatch = errors.New("pcgw: no page match")

// ErrRateLimited is returned on HTTP 429/5xx with no cached answer.
var ErrRateLimited = errors.New("pcgw: rate limited")

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

// SearchTitle resolves a free-text title to the wiki's canonical page
// title (first opensearch result). Answers come from the disk cache when
// fresh; empty results are cached as negatives.
func (c *Client) SearchTitle(ctx context.Context, term string) (title string, live bool, err error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return "", false, errors.New("pcgw: empty term")
	}
	if hit, ok := c.readCache("title", term); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return "", false, fmt.Errorf("%w for %q (cached)", ErrNoMatch, term)
		}
		return hit.Value, false, nil
	}
	var payload []interface{}
	if err := c.get(ctx, "/w/api.php?action=opensearch&search="+url.QueryEscape(term)+"&redirects=resolve&format=json", &payload); err != nil {
		return "", true, err
	}
	if len(payload) < 2 {
		c.writeCache("title", term, cachedValue{FetchedAt: c.now(), NoMatch: true})
		return "", true, fmt.Errorf("%w for %q (empty results)", ErrNoMatch, term)
	}
	names, ok := payload[1].([]interface{})
	if !ok || len(names) == 0 {
		c.writeCache("title", term, cachedValue{FetchedAt: c.now(), NoMatch: true})
		return "", true, fmt.Errorf("%w for %q (empty results)", ErrNoMatch, term)
	}
	name, _ := names[0].(string)
	if name == "" {
		c.writeCache("title", term, cachedValue{FetchedAt: c.now(), NoMatch: true})
		return "", true, fmt.Errorf("%w for %q (empty results)", ErrNoMatch, term)
	}
	c.writeCache("title", term, cachedValue{Value: name, FetchedAt: c.now()})
	return name, true, nil
}

// TitleBySteamAppID reverse-looks-up a wiki page title by Steam appid
// (Cargo HOLDS on Infobox_game.Steam_AppID).
func (c *Client) TitleBySteamAppID(ctx context.Context, appid string) (title string, live bool, err error) {
	appid = strings.TrimSpace(appid)
	if appid == "" {
		return "", false, errors.New("pcgw: empty appid")
	}
	if hit, ok := c.readCache("appid", appid); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return "", false, fmt.Errorf("%w for appid %s (cached)", ErrNoMatch, appid)
		}
		return hit.Value, false, nil
	}
	q := "/w/api.php?action=cargoquery&tables=Infobox_game" +
		"&fields=Infobox_game._pageName=Page" +
		"&where=" + url.QueryEscape("Infobox_game.Steam_AppID HOLDS \""+appid+"\"") +
		"&format=json"
	var payload struct {
		CargoQuery []struct {
			Title struct {
				Page string `json:"Page"`
			} `json:"title"`
		} `json:"cargoquery"`
	}
	if err := c.get(ctx, q, &payload); err != nil {
		return "", true, err
	}
	if len(payload.CargoQuery) == 0 || payload.CargoQuery[0].Title.Page == "" {
		c.writeCache("appid", appid, cachedValue{FetchedAt: c.now(), NoMatch: true})
		return "", true, fmt.Errorf("%w for appid %s", ErrNoMatch, appid)
	}
	name := payload.CargoQuery[0].Title.Page
	c.writeCache("appid", appid, cachedValue{Value: name, FetchedAt: c.now()})
	return name, true, nil
}

// get performs one paced, cached-cooldown-aware JSON GET.
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
	return json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(out)
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
	LastAttempt time.Time `json:"last_attempt"`
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
	return c.now().Sub(state.LastAttempt) < cooldown
}

func (c *Client) writeCooldown(t time.Time) error {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cooldownState{LastAttempt: t})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.cacheDir, cooldownFile), data, 0o644)
}

// cachedValue is one persisted lookup result (title text or a negative).
type cachedValue struct {
	Value     string    `json:"value,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	NoMatch   bool      `json:"no_match,omitempty"`
}

func (c *Client) cacheFile(kind, key string) string {
	sum := sha256.Sum256([]byte(kind + ":" + strings.ToLower(strings.TrimSpace(key))))
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
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(cv)
	if err != nil {
		return
	}
	_ = os.WriteFile(c.cacheFile(kind, key), data, 0o644)
}

// CoverFile resolves a wiki page to its infobox cover image filename
// (Cargo Infobox_game.Cover). Empty when the page has no cover.
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
	q := "/w/api.php?action=cargoquery&tables=Infobox_game" +
		"&fields=" + url.QueryEscape("Infobox_game._pageName=Page,Infobox_game.Cover") +
		"&where=" + url.QueryEscape("Infobox_game._pageName=\""+pageTitle+"\"") +
		"&format=json"
	var payload struct {
		CargoQuery []struct {
			Title struct {
				Cover string `json:"Cover"`
			} `json:"title"`
		} `json:"cargoquery"`
	}
	if err := c.get(ctx, q, &payload); err != nil {
		return "", true, err
	}
	if len(payload.CargoQuery) == 0 || payload.CargoQuery[0].Title.Cover == "" {
		c.writeCache("cover", key, cachedValue{FetchedAt: c.now(), NoMatch: true})
		return "", true, fmt.Errorf("%w for %q", ErrNoMatch, pageTitle)
	}
	fileName = payload.CargoQuery[0].Title.Cover
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
