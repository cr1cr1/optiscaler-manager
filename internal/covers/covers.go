// Package covers fetches and caches game cover art. Chain: Steam CDN by
// appid → Steam store search (name → appid) → generated placeholder.
package covers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/gid"
	"github.com/cr1cr1/optiscaler-manager/internal/pcgw"
)

const (
	steamCDN    = "https://cdn.cloudflare.steamstatic.com/steam/apps/%s/library_600x900.jpg"
	steamSearch = "https://store.steampowered.com/api/storesearch/"
)

// Covers resolves and caches cover art under cacheDir.
type Covers struct {
	http     *http.Client
	cacheDir string

	// PCGW, when non-nil, is the portrait-art fallback for games Steam's
	// CDN has no poster for (and the only art source for games not on
	// Steam at all). Wiki box art is poster-like; the Steam hero banner
	// (landscape) is the last resort before the placeholder.
	PCGW *pcgw.Client

	// UserAgent identifies every outbound request; the wiki's hosts
	// reject requests without a descriptive UA (403), and Go's default
	// UA string is rejected too.
	UserAgent string

	// Overridable for tests.
	cdnBase    string
	searchBase string
}

// New returns a Covers using httpClient (nil → http.DefaultClient) with
// cacheDir as the on-disk cache root.
func New(httpClient *http.Client, cacheDir string) *Covers {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Covers{
		http:       httpClient,
		cacheDir:   cacheDir,
		UserAgent:  "optiscaler-manager/dev (https://github.com/cr1cr1/optiscaler-manager)",
		cdnBase:    steamCDN,
		searchBase: steamSearch,
	}
}

// NewWithBase is New with explicit service base URLs (tests, mirrors).
// cdnBase must contain one %s verb for the appid.
func NewWithBase(httpClient *http.Client, cacheDir, cdnBase, searchBase string) *Covers {
	c := New(httpClient, cacheDir)
	c.cdnBase = cdnBase
	c.searchBase = searchBase
	return c
}

// Cover returns the local path of the game's cover image, downloading and
// caching it if needed. On any miss it returns the shared placeholder (never
// an error for a missing cover — art is decorative). The appid path goes
// straight to the CDN (no search); a title search only fires without one,
// and its candidates are scored — a wrong cover is worse than no cover.
func (c *Covers) Cover(ctx context.Context, appID, name string) (string, error) {
	if sanitized := sanitize(appID); sanitized != "" {
		cached := filepath.Join(c.cacheDir, sanitized+".img")
		if _, err := os.Stat(cached); err == nil {
			return cached, nil
		}
		if c.recentMiss(sanitized) {
			return c.placeholder()
		}
		if err := c.fetch(ctx, fmt.Sprintf(c.artURL("library_600x900.jpg"), url.PathEscape(sanitized)), cached); err == nil {
			return cached, nil
		}
		if p, ok := c.fromPCGW(ctx, sanitized, name); ok {
			return p, nil
		}
		if err := c.fetch(ctx, fmt.Sprintf(c.artURL("library_hero.jpg"), url.PathEscape(sanitized)), cached); err == nil {
			return cached, nil
		}
		c.markMiss(sanitized)
	}

	if name != "" {
		if id, err := c.searchAppID(ctx, name); err == nil && id != "" {
			cached := filepath.Join(c.cacheDir, id+".img")
			if err := c.fetch(ctx, fmt.Sprintf(c.artURL("library_600x900.jpg"), url.PathEscape(id)), cached); err == nil {
				return cached, nil
			}
			if p, ok := c.fromPCGW(ctx, id, name); ok {
				return p, nil
			}
			if err := c.fetch(ctx, fmt.Sprintf(c.artURL("library_hero.jpg"), url.PathEscape(id)), cached); err == nil {
				return cached, nil
			}
		} else if p, ok := c.fromPCGW(ctx, "", name); ok {
			return p, nil
		}
	}

	return c.placeholder()
}

// artURL formats one of the CDN art variants for an appid. cdnBase carries
// the portrait pattern; the hero banner swaps the filename.
func (c *Covers) artURL(variant string) string {
	return strings.Replace(c.cdnBase, "library_600x900.jpg", variant, 1)
}

// missTTL is how long a known-artless appid is not re-fetched.
const missTTL = 7 * 24 * time.Hour

func (c *Covers) recentMiss(appid string) bool {
	st, err := os.Stat(filepath.Join(c.cacheDir, appid+".miss"))
	if err != nil {
		return false
	}
	return time.Since(st.ModTime()) < missTTL
}

func (c *Covers) markMiss(appid string) {
	if err := c.ensureCacheDir(); err != nil {
		return
	}
	f, err := os.Create(filepath.Join(c.cacheDir, appid+".miss"))
	if err == nil {
		_ = f.Close()
	}
}

// searchAppID resolves a game name to a Steam appid via the store search API.
// The first hit is NOT automatically the game: candidates are scored
// (normalized exact or near-equal, PC bonus, edition penalty) and only an
// accepted score binds — anything weaker means no cover rather than the
// wrong one.
func (c *Covers) searchAppID(ctx context.Context, name string) (string, error) {
	u := c.searchBase + "?term=" + url.QueryEscape(name) + "&cc=us&l=en"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("store search: HTTP %d", resp.StatusCode)
	}
	var result struct {
		Items []struct {
			ID        json.Number `json:"id"`
			Name      string      `json:"name"`
			Platforms struct {
				Windows bool `json:"windows"`
			} `json:"platforms"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	for _, item := range result.Items {
		if gid.Accept(gid.Score(name, item.Name, item.Platforms.Windows), false) {
			return sanitize(item.ID.String()), nil
		}
	}
	return "", nil
}

// fetch downloads url to dest atomically (temp + rename), rejecting non-200
// and non-image responses.
func (c *Covers) fetch(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cover fetch: HTTP %d", resp.StatusCode)
	}
	if err := c.ensureCacheDir(); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.cacheDir, ".dl-*")
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, io.LimitReader(resp.Body, 32<<20)); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), dest)
}

// placeholder writes (once) and returns a simple dark tile PNG.
// ensureCacheDir creates the cache directory, but only when its parent
// still exists: a vanished parent means the process (or test) is tearing
// down, and recreating the tree would race that removal.
func (c *Covers) ensureCacheDir() error {
	if _, err := os.Stat(c.cacheDir); err == nil {
		return nil
	}
	if _, err := os.Stat(filepath.Dir(c.cacheDir)); err != nil {
		return fmt.Errorf("cover cache parent gone: %w", err)
	}
	return os.MkdirAll(c.cacheDir, 0o755)
}

func (c *Covers) placeholder() (string, error) {
	p := filepath.Join(c.cacheDir, "_placeholder.png")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	if err := c.ensureCacheDir(); err != nil {
		return "", err
	}
	img := image.NewRGBA(image.Rect(0, 0, 60, 90))
	bg := color.RGBA{24, 24, 32, 255}
	for y := 0; y < 90; y++ {
		for x := 0; x < 60; x++ {
			img.Set(x, y, bg)
		}
	}
	f, err := os.Create(p)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		return "", err
	}
	return p, nil
}

// sanitize keeps only digits from an appid, so manifest data can never
// escape the cache directory or build a hostile URL.
func sanitize(appID string) string {
	var b strings.Builder
	for _, r := range appID {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fromPCGW resolves box art via PCGamingWiki: page by Steam appid when
// known, else by title (scored against the candidate). Returns the cached
// image path on success.
func (c *Covers) fromPCGW(ctx context.Context, appID, name string) (string, bool) {
	if c.PCGW == nil {
		return "", false
	}
	page := ""
	if appID != "" {
		if t, _, err := c.PCGW.TitleBySteamAppID(ctx, appID); err == nil {
			page = t
		}
	}
	if page == "" && name != "" {
		if t, _, err := c.PCGW.SearchTitle(ctx, name); err == nil && t != "" && gid.Accept(gid.Score(name, t, true), false) {
			page = t
		}
	}
	if page == "" {
		return "", false
	}
	file, _, err := c.PCGW.CoverFile(ctx, page)
	if err != nil || file == "" {
		return "", false
	}
	thumb, _, err := c.PCGW.ImageThumbURL(ctx, file, 600)
	if err != nil || thumb == "" {
		return "", false
	}
	dest := filepath.Join(c.cacheDir, appID+".img")
	if appID == "" {
		sum := sha256.Sum256([]byte("pcgw:" + strings.ToLower(page)))
		dest = filepath.Join(c.cacheDir, "pcgw_"+hex.EncodeToString(sum[:])[:16]+".img")
	}
	if err := c.fetch(ctx, thumb, dest); err == nil {
		return dest, true
	}
	return "", false
}
