// Package sgdb resolves game grid (poster) art against SteamGridDB, the
// cover source of last resort for games Steam's CDN has no art for
// (unreleased/asset-less games) and for names Steam's search cannot
// match (issue 024). Authenticated with a personal API key (Bearer).
// The client mirrors the pcgw package's discipline: a 30d disk cache
// with negatives, and API failures (non-200, or 200 with
// "success":false) as live errors that are never cached.
package sgdb

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
	"strconv"
	"strings"
	"time"
)

// ErrNoMatch is returned when SteamGridDB has no game or no grids for
// the query. It is cached as a negative.
var ErrNoMatch = errors.New("sgdb: no match")

// ErrAPI is returned on any API failure (non-200 status, or a 200
// envelope with "success":false — e.g. an invalid key). It is a live
// error and is never cached as a negative.
var ErrAPI = errors.New("sgdb: API error")

const (
	cacheTTL       = 30 * 24 * time.Hour
	maxBodyBytes   = 1 << 20
	defaultBaseURL = "https://www.steamgriddb.com/api/v2"
)

// Game is one SteamGridDB game record.
type Game struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Client queries the SteamGridDB API v2.
type Client struct {
	http      *http.Client
	cacheDir  string
	baseURL   string
	key       string
	userAgent string
	now       func() time.Time
}

// New returns a Client authenticated with the user's personal API key.
func New(httpClient *http.Client, cacheDir, key, version string) *Client {
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
		key:       key,
		userAgent: "optiscaler-manager/" + version + " (https://github.com/cr1cr1/optiscaler-manager)",
		now:       time.Now,
	}
}

// NewWithBaseURL is New with an explicit API host, for tests.
func NewWithBaseURL(httpClient *http.Client, cacheDir, key, baseURL, version string) *Client {
	c := New(httpClient, cacheDir, key, version)
	c.baseURL = baseURL
	return c
}

// GameBySteamAppID resolves a Steam appid to the SteamGridDB game.
// Unknown appids (404) are cached as negatives.
func (c *Client) GameBySteamAppID(ctx context.Context, appid string) (game Game, live bool, err error) {
	appid = strings.TrimSpace(appid)
	if appid == "" {
		return Game{}, false, errors.New("sgdb: empty appid")
	}
	key := "steam:" + appid
	if hit, ok := c.readCache(key); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return Game{}, false, fmt.Errorf("%w for appid %s (cached)", ErrNoMatch, appid)
		}
		return Game{ID: hit.ID, Name: hit.Name}, false, nil
	}
	env, err := c.get(ctx, "/games/steam/"+url.PathEscape(appid))
	if err != nil {
		if errors.Is(err, ErrNoMatch) {
			c.writeCache(key, cachedEntry{FetchedAt: c.now(), NoMatch: true})
			return Game{}, true, fmt.Errorf("%w for appid %s", ErrNoMatch, appid)
		}
		return Game{}, true, err
	}
	if err := json.Unmarshal(env.Data, &game); err != nil || game.ID == 0 {
		c.writeCache(key, cachedEntry{FetchedAt: c.now(), NoMatch: true})
		return Game{}, true, fmt.Errorf("%w for appid %s", ErrNoMatch, appid)
	}
	c.writeCache(key, cachedEntry{ID: game.ID, Name: game.Name, FetchedAt: c.now()})
	return game, true, nil
}

// SearchGame resolves a free-text title to the top autocomplete hit.
// SteamGridDB's search is alias-aware; the caller scores/gates the hit.
// Empty results are cached as negatives.
func (c *Client) SearchGame(ctx context.Context, term string) (game Game, live bool, err error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return Game{}, false, errors.New("sgdb: empty term")
	}
	key := "search:" + strings.ToLower(term)
	if hit, ok := c.readCache(key); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return Game{}, false, fmt.Errorf("%w for %q (cached)", ErrNoMatch, term)
		}
		return Game{ID: hit.ID, Name: hit.Name}, false, nil
	}
	env, err := c.get(ctx, "/search/autocomplete/"+url.PathEscape(term))
	if err != nil {
		if errors.Is(err, ErrNoMatch) {
			c.writeCache(key, cachedEntry{FetchedAt: c.now(), NoMatch: true})
			return Game{}, true, fmt.Errorf("%w for %q", ErrNoMatch, term)
		}
		return Game{}, true, err
	}
	var games []Game
	if err := json.Unmarshal(env.Data, &games); err != nil || len(games) == 0 || games[0].ID == 0 {
		c.writeCache(key, cachedEntry{FetchedAt: c.now(), NoMatch: true})
		return Game{}, true, fmt.Errorf("%w for %q", ErrNoMatch, term)
	}
	game = games[0]
	c.writeCache(key, cachedEntry{ID: game.ID, Name: game.Name, FetchedAt: c.now()})
	return game, true, nil
}

// GridURL resolves a game to its best (score-sorted first) 600x900
// static grid image URL. Games without grids are cached as negatives.
func (c *Client) GridURL(ctx context.Context, gameID int64) (gridURL string, live bool, err error) {
	if gameID == 0 {
		return "", false, errors.New("sgdb: zero game id")
	}
	key := "grid:" + strconv.FormatInt(gameID, 10)
	if hit, ok := c.readCache(key); ok && c.now().Sub(hit.FetchedAt) < cacheTTL {
		if hit.NoMatch {
			return "", false, fmt.Errorf("%w for game %d (cached)", ErrNoMatch, gameID)
		}
		return hit.Value, false, nil
	}
	env, err := c.get(ctx, "/grids/game/"+strconv.FormatInt(gameID, 10)+"?dimensions=600x900&types=static")
	if err != nil {
		if errors.Is(err, ErrNoMatch) {
			c.writeCache(key, cachedEntry{FetchedAt: c.now(), NoMatch: true})
			return "", true, fmt.Errorf("%w for game %d", ErrNoMatch, gameID)
		}
		return "", true, err
	}
	var grids []struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(env.Data, &grids); err != nil || len(grids) == 0 || grids[0].URL == "" {
		c.writeCache(key, cachedEntry{FetchedAt: c.now(), NoMatch: true})
		return "", true, fmt.Errorf("%w for game %d", ErrNoMatch, gameID)
	}
	c.writeCache(key, cachedEntry{Value: grids[0].URL, FetchedAt: c.now()})
	return grids[0].URL, true, nil
}

// envelope is the SteamGridDB response wrapper: data's shape varies per
// endpoint, so callers unmarshal it themselves.
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Errors  []string        `json:"errors"`
}

// get performs one authenticated JSON GET. HTTP 404 (unknown game) is
// ErrNoMatch; any other non-200, or a 200 with success:false, is ErrAPI.
func (c *Client) get(ctx context.Context, path string) (envelope, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return envelope{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return envelope{}, fmt.Errorf("sgdb: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return envelope{}, fmt.Errorf("sgdb: read body: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return envelope{}, ErrNoMatch
	}
	if resp.StatusCode != http.StatusOK {
		return envelope{}, fmt.Errorf("%w (HTTP %d)", ErrAPI, resp.StatusCode)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return envelope{}, fmt.Errorf("sgdb: decode: %w", err)
	}
	if !env.Success {
		return envelope{}, fmt.Errorf("%w: %s", ErrAPI, strings.Join(env.Errors, "; "))
	}
	return env, nil
}

// cachedEntry is one persisted lookup result (a game id+name, a grid
// URL, or a negative).
type cachedEntry struct {
	Value     string    `json:"value,omitempty"`
	ID        int64     `json:"id,omitempty"`
	Name      string    `json:"name,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	NoMatch   bool      `json:"no_match,omitempty"`
}

func (c *Client) cacheFile(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.cacheDir, "sgdb_"+hex.EncodeToString(sum[:])[:16]+".json")
}

func (c *Client) readCache(key string) (cachedEntry, bool) {
	var ce cachedEntry
	data, err := os.ReadFile(c.cacheFile(key))
	if err != nil {
		return ce, false
	}
	if err := json.Unmarshal(data, &ce); err != nil {
		return cachedEntry{}, false
	}
	return ce, true
}

func (c *Client) writeCache(key string, ce cachedEntry) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(ce)
	if err != nil {
		return
	}
	_ = os.WriteFile(c.cacheFile(key), data, 0o644)
}
