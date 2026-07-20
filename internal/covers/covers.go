// Package covers fetches and caches game cover art. Chain: Steam CDN by
// appid → Steam store search (name → appid) → generated placeholder.
package covers

import (
	"context"
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
)

const (
	steamCDN    = "https://cdn.cloudflare.steamstatic.com/steam/apps/%s/library_600x900.jpg"
	steamSearch = "https://store.steampowered.com/api/storesearch/"
)

// Covers resolves and caches cover art under cacheDir.
type Covers struct {
	http     *http.Client
	cacheDir string

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
	return &Covers{http: httpClient, cacheDir: cacheDir, cdnBase: steamCDN, searchBase: steamSearch}
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
// an error for a missing cover — art is decorative).
func (c *Covers) Cover(ctx context.Context, appID, name string) (string, error) {
	if sanitized := sanitize(appID); sanitized != "" {
		cached := filepath.Join(c.cacheDir, sanitized+".img")
		if _, err := os.Stat(cached); err == nil {
			return cached, nil
		}
		if err := c.fetch(ctx, fmt.Sprintf(c.cdnBase, url.PathEscape(sanitized)), cached); err == nil {
			return cached, nil
		}
	}

	if name != "" {
		if id, err := c.searchAppID(ctx, name); err == nil && id != "" {
			cached := filepath.Join(c.cacheDir, id+".img")
			if err := c.fetch(ctx, fmt.Sprintf(c.cdnBase, url.PathEscape(id)), cached); err == nil {
				return cached, nil
			}
		}
	}

	return c.placeholder()
}

// searchAppID resolves a game name to a Steam appid via the store search API.
func (c *Covers) searchAppID(ctx context.Context, name string) (string, error) {
	u := c.searchBase + "?term=" + url.QueryEscape(name) + "&cc=us&l=en"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
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
			ID   json.Number `json:"id"`
			Name string      `json:"name"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Items) == 0 {
		return "", nil
	}
	return sanitize(result.Items[0].ID.String()), nil
}

// fetch downloads url to dest atomically (temp + rename), rejecting non-200
// and non-image responses.
func (c *Covers) fetch(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
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
