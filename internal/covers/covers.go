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
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/cr1cr1/optiscaler-manager/internal/gid"
	"github.com/cr1cr1/optiscaler-manager/internal/pcgw"
	"github.com/cr1cr1/optiscaler-manager/internal/sgdb"
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

	// SGDB, when non-nil (a SteamGridDB API key is configured), supplies
	// grid art for games Steam's CDN has no poster for (unreleased or
	// asset-less appids) and for names Steam's own search cannot match.
	// It sits between the Steam CDN and the wiki: official Steam art
	// still wins, community grids beat the landscape hero.
	SGDB *sgdb.Client

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
// straight to the CDN (no search); a title search always runs when the
// appid path produced nothing — a known-artless appid (recent miss marker)
// only skips the CDN retry, never the search, because a wrong appid guess
// must not bury a resolvable title. Search candidates are scored and the
// best-probability one binds: a wrong cover is worse than no cover.
func (c *Covers) Cover(ctx context.Context, appID, name string) (string, error) {
	if sanitized := sanitize(appID); sanitized != "" {
		cached := filepath.Join(c.cacheDir, sanitized+".img")
		if _, err := os.Stat(cached); err == nil {
			normalizeCover(cached) // legacy scrub (issue 025)
			return cached, nil
		}
		if !c.recentMiss(sanitized) {
			if err := c.fetch(ctx, fmt.Sprintf(c.artURL("library_600x900.jpg"), url.PathEscape(sanitized)), cached); err == nil {
				return cached, nil
			}
			if p, ok := c.fromSGDB(ctx, sanitized, name); ok {
				return p, nil
			}
			if p, ok := c.fromPCGW(ctx, sanitized, name); ok {
				return p, nil
			}
			if err := c.fetch(ctx, fmt.Sprintf(c.artURL("library_hero.jpg"), url.PathEscape(sanitized)), cached); err == nil {
				return cached, nil
			}
			c.markMiss(sanitized)
		}
	}

	if name != "" {
		if id, err := c.searchAppID(ctx, name); err == nil && id != "" {
			cached := filepath.Join(c.cacheDir, id+".img")
			if err := c.fetch(ctx, fmt.Sprintf(c.artURL("library_600x900.jpg"), url.PathEscape(id)), cached); err == nil {
				return cached, nil
			}
			if p, ok := c.fromSGDB(ctx, id, name); ok {
				return p, nil
			}
			if p, ok := c.fromPCGW(ctx, id, name); ok {
				return p, nil
			}
			if err := c.fetch(ctx, fmt.Sprintf(c.artURL("library_hero.jpg"), url.PathEscape(id)), cached); err == nil {
				return cached, nil
			}
		} else if p, ok := c.fromSGDB(ctx, "", name); ok {
			return p, nil
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
//
// ponytail: mtime-based negative cache (ceiling: clock skew or manual
// cache-dir edits can extend/shrink the TTL); upgrade path: JSON cache
// records like internal/steam and internal/protondb use.
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
// Candidates are scored (normalized exact or near-equal, PC bonus, edition
// penalty) and the BEST score above the acceptance threshold binds — not the
// first acceptable hit; anything weaker means no cover rather than the wrong
// one.
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
	bestID, bestScore := "", -1
	for _, item := range result.Items {
		if score := gid.Score(name, item.Name, item.Platforms.Windows); score > bestScore {
			bestID, bestScore = sanitize(item.ID.String()), score
		}
	}
	if bestID != "" && gid.Accept(bestScore, false) {
		return bestID, nil
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
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return err
	}
	normalizeCover(dest) // aspect invariant (issue 025)
	return nil
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

// fromPCGW resolves box art via PCGamingWiki by title: every opensearch
// hit is scored and the best accepted page wins (the wiki ranks loosely —
// first is not always right). The appid only names the cache file; the
// wiki's anonymous API has no appid reverse lookup since the Cargo module
// was restricted (issue 024). Returns the cached image path on success.
func (c *Covers) fromPCGW(ctx context.Context, appID, name string) (string, bool) {
	if c.PCGW == nil || name == "" {
		return "", false
	}
	titles, _, err := c.PCGW.SearchTitles(ctx, name)
	if err != nil {
		return "", false
	}
	page := gid.BestAccepted(name, titles, true)
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

// fromSGDB resolves grid art via SteamGridDB: by Steam appid when known
// (precise), else by the alias-aware autocomplete gated by
// sgdbNameAccepts. Returns the cached image path on success.
func (c *Covers) fromSGDB(ctx context.Context, appID, name string) (string, bool) {
	if c.SGDB == nil {
		return "", false
	}
	var id int64
	if appID != "" {
		if g, _, err := c.SGDB.GameBySteamAppID(ctx, appID); err == nil {
			id = g.ID
		}
	}
	if id == 0 && name != "" {
		if g, _, err := c.SGDB.SearchGame(ctx, name); err == nil && sgdbNameAccepts(name, g.Name) {
			id = g.ID
		}
	}
	if id == 0 {
		return "", false
	}
	u, _, err := c.SGDB.GridURL(ctx, id)
	if err != nil || u == "" {
		return "", false
	}
	dest := filepath.Join(c.cacheDir, appID+".img")
	if appID == "" {
		sum := sha256.Sum256([]byte(fmt.Sprintf("sgdb:%d", id)))
		dest = filepath.Join(c.cacheDir, "sgdb_"+hex.EncodeToString(sum[:])[:16]+".img")
	}
	if err := c.fetch(ctx, u, dest); err == nil {
		return dest, true
	}
	return "", false
}

// sgdbNameAccepts gates SGDB's alias-aware top autocomplete hit. The
// strict scorer stands first (gid.Accept); otherwise a scoped subset
// rule applies for cover picking only: every normalized candidate token
// must appear in the hit, and the hit must not add numeral tokens —
// "the witcher 3" ⊆ "the witcher 3 wild hunt" binds, "frostpunk" ⊄
// "frostpunk 2" refuses. Identification (titles, appid binding) never
// uses this rule: a wrong COVER is tolerable here, a wrong identity is
// not (issue 024).
func sgdbNameAccepts(cand, hit string) bool {
	if gid.Accept(gid.Score(cand, hit, true), false) {
		return true
	}
	c, h := gid.Normalize(cand), gid.Normalize(hit)
	if c == "" || h == "" {
		return false
	}
	extra := map[string]int{}
	for _, t := range strings.Fields(h) {
		extra[t]++
	}
	for _, t := range strings.Fields(c) {
		if extra[t] == 0 {
			return false
		}
		extra[t]--
	}
	for t, n := range extra {
		if n > 0 && strings.ContainsAny(t, "0123456789") {
			return false
		}
	}
	return true
}

// The card aspect is 2:3 portrait (Steam library_600x900, wiki box art,
// SGDB grids). Every image the cache stores or returns must hold it:
// the renderer (vendored ImageFill) is deliberately aspect-blind, so the
// cache is the one place that can guarantee no art is ever stretched —
// whatever any current or future source produces (issue 025).
//
// normalizeCover enforces the invariant on one cached file: art already
// at ~2:3 passes through byte-identical (no re-encode quality loss);
// anything else is center-cropped (hero banners are center-composed) and
// re-encoded in place — JPEG stays JPEG, everything else becomes PNG
// (webp has no stdlib encoder). Undecodable content is left alone: the
// renderer already tolerates unloadable files. It runs on every fetch
// (write path) and on every cached hit (legacy scrub), idempotently.
func normalizeCover(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	cfg, _, err := image.DecodeConfig(f)
	_ = f.Close()
	if err != nil || cfg.Width < 2 || cfg.Height < 2 {
		return
	}
	// ~2:3 passthrough: |3w − 2h| within 1% of 3w.
	if d := 3*cfg.Width - 2*cfg.Height; d > -cfg.Width/33 && d < cfg.Width/33 {
		return
	}
	f, err = os.Open(path)
	if err != nil {
		return
	}
	src, format, err := image.Decode(f)
	_ = f.Close()
	if err != nil {
		return
	}
	cropped := centerCrop23(src)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".norm-*")
	if err != nil {
		return
	}
	enc := func() error {
		if format == "jpeg" {
			return jpeg.Encode(tmp, cropped, &jpeg.Options{Quality: 90})
		}
		return png.Encode(tmp, cropped)
	}
	if err := enc(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	_ = os.Rename(tmp.Name(), path)
}

// centerCrop23 returns src center-cropped to exactly 2:3 portrait.
func centerCrop23(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	var r image.Rectangle
	if 3*w > 2*h {
		// Too wide (hero banners): crop left and right equally.
		nw := 2 * h / 3
		x0 := b.Min.X + (w-nw)/2
		r = image.Rect(x0, b.Min.Y, x0+nw, b.Max.Y)
	} else {
		// Too tall: crop top and bottom equally.
		nh := 3 * w / 2
		y0 := b.Min.Y + (h-nh)/2
		r = image.Rect(b.Min.X, y0, b.Max.X, y0+nh)
	}
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	return dst
}
