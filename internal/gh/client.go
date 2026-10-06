// Package gh resolves OptiScaler releases and downloadable bundle assets
// from the GitHub API, with a client-side rate-limit cooldown and a
// persistent releases cache.
//
// Scope (docs/scope.md): bundle-only, unauthenticated API (60 req/h),
// no retry/backoff — a 15-minute cooldown replaces it. The GUI surfaces
// the fromCache signal as a warning; this package only exposes it.
package gh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// ErrRateLimited is returned when the GitHub API rate limit is active
// (HTTP 403 with X-RateLimit-Remaining: 0, or HTTP 429) and no cached
// releases.json is available to serve from.
var ErrRateLimited = errors.New("gh: github API rate limited")

const (
	// cooldown is the client-side back-off window started by an ANSWERED
	// API call (success or rate limit; never a transport failure). Inside
	// the window the cache is served as-is and never auto-refreshes.
	cooldown = 15 * time.Minute

	defaultBaseURL = "https://api.github.com"

	releasesCacheFile = "releases.json"
	cooldownFile      = "cooldown.json"

	// Upstream distribution source: the repository every legacy client
	// talks to, and the glob matching its bundle assets. The glob is a
	// path.Match pattern, never an exact filename — names embed a date
	// and a _MM marker (docs/scope.md). Fork clients override both.
	defaultSlug         = "optiscaler/OptiScaler"
	defaultAssetPattern = "Optiscaler_*.7z"
)

// Release mirrors the GitHub API release JSON fields actually used.
type Release struct {
	TagName    string  `json:"tag_name"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Asset mirrors the GitHub API asset JSON fields actually used.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Client resolves and downloads OptiScaler release assets.
type Client struct {
	http     *http.Client
	cacheDir string

	// baseURL is unexported so tests can point at httptest servers.
	baseURL string

	// slug is the owner/repo release source; assetPattern is the
	// path.Match glob selectAsset applies. Both default to upstream.
	slug         string
	assetPattern string

	// now is a clock hook for tests (cooldown expiry).
	now func() time.Time

	// downloadURLs maps asset name → URL from the last fetch or cache
	// load. domain.ResolvedAsset deliberately carries no URL; Download
	// resolves it here. mu guards the map AND the releases() sequence that
	// refills it: a startup scan and the startup preload can Resolve at the
	// same instant, and without the lock both would pass the cooldown check
	// (or read the map mid-index) and fetch the release list twice.
	mu sync.Mutex

	// lastFetchAt is the wall time of the last successful LIVE fetch by
	// THIS process (zero until one happens). A resolve served from the
	// cache during the cooldown window is still provably fresh while this
	// is recent — CacheFresh.
	lastFetchAt  time.Time
	downloadURLs map[string]string
}

// New returns a Client for the upstream OptiScaler repository. A nil
// httpClient uses http.DefaultClient.
func New(httpClient *http.Client, cacheDir string) *Client {
	return NewFork(httpClient, cacheDir, "", "")
}

// NewFork is New pointed at an arbitrary fork: slug is the owner/repo
// release source and assetPattern the path.Match glob for its bundle
// asset. Empty values fall back to upstream.
func NewFork(httpClient *http.Client, cacheDir, slug, assetPattern string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if slug == "" {
		slug = defaultSlug
	}
	if assetPattern == "" {
		assetPattern = defaultAssetPattern
	}
	return &Client{
		http:         httpClient,
		cacheDir:     cacheDir,
		baseURL:      defaultBaseURL,
		slug:         slug,
		assetPattern: assetPattern,
		now:          time.Now,
		downloadURLs: make(map[string]string),
	}
}

// NewWithBaseURL is New with an explicit API base URL, for tests and
// OM_GH_BASE_URL-style overrides.
func NewWithBaseURL(httpClient *http.Client, cacheDir, baseURL string) *Client {
	return NewForkWithBaseURL(httpClient, cacheDir, baseURL, "", "")
}

// NewForkWithBaseURL is NewFork with an explicit API base URL.
func NewForkWithBaseURL(httpClient *http.Client, cacheDir, baseURL, slug, assetPattern string) *Client {
	c := NewFork(httpClient, cacheDir, slug, assetPattern)
	c.baseURL = baseURL
	return c
}

// releasesURL is the release-list endpoint for the client's repository.
func (c *Client) releasesURL() string {
	return c.baseURL + "/repos/" + c.slug + "/releases?per_page=30"
}

// Resolve maps a requested version to a concrete release asset.
//
// requested semantics: "latest" → first non-prerelease release; otherwise
// an exact tag match. Unknown values fail loud with an error naming the
// requested value.
//
// Asset selection is the client's path.Match glob (case-sensitive, as the
// distribution publishes — upstream "Optiscaler_*.7z", forks their own).
// Multiple matches pick the lexicographically first; the chosen name is
// reported in ResolvedAsset.AssetName for the caller to log. Zero matches
// is an error.
//
// ResolvedAsset.SHA256 is left empty: the GitHub API provides no digest;
// it is filled at download time (see Download).
//
// fromCache reports that resolution was served from the persisted
// releases.json because the client is inside the rate-limit cooldown (or
// the API just rate-limited us). Callers (GUI) must surface this as a
// warning; requested and resolved values travel separately so the caller
// can record both (docs/safety.md).
func (c *Client) Resolve(ctx context.Context, requested string) (resolved domain.ResolvedAsset, fromCache bool, err error) {
	if requested == "" {
		return domain.ResolvedAsset{}, false, fmt.Errorf("gh: empty requested version")
	}

	releases, fromCache, err := c.releases(ctx)
	if err != nil {
		return domain.ResolvedAsset{}, fromCache, err
	}

	rel, err := selectRelease(releases, requested)
	if err != nil {
		return domain.ResolvedAsset{}, fromCache, err
	}

	asset, err := c.selectAsset(rel)
	if err != nil {
		return domain.ResolvedAsset{}, fromCache, err
	}

	return domain.ResolvedAsset{
		AssetName: asset.Name,
		Version:   rel.TagName,
		SHA256:    "", // no digest in the API; computed by Download
	}, fromCache, nil
}

// releases returns the release list, honoring the cooldown and cache. The
// whole sequence runs under mu so concurrent Resolves (a startup scan and
// the startup preload firing at once) share one fetch instead of racing
// the cooldown check, the cache write, and the download-URL index.
func (c *Client) releases(ctx context.Context) ([]Release, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.inCooldown() {
		if cached, err := c.readCache(); err == nil {
			return cached, true, nil
		}
		return nil, false, fmt.Errorf("%w (cooldown active, no cached releases)", ErrRateLimited)
	}

	releases, err := c.fetch(ctx)
	if err != nil {
		if errors.Is(err, ErrRateLimited) {
			if cached, cerr := c.readCache(); cerr == nil {
				return cached, true, nil
			}
		}
		return nil, false, err
	}
	return releases, false, nil
}

// fetch performs the API request and persists a successful response to
// releases.json. The cooldown starts only when the API actually answered:
// a rate-limited response (back off) or a success (serve the just-written
// cache for the window). A transport failure, a non-200, or a decode
// failure records nothing — the next resolve retries the live API instead
// of being locked out under a misleading rate-limit error (scope H4).
func (c *Client) fetch(ctx context.Context) ([]Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.releasesURL(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gh: releases request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if isRateLimited(resp) {
		// The API said no: start the back-off window, then serve the
		// (possibly stale) cache for its duration.
		_ = c.writeCooldown(c.now())
		return nil, fmt.Errorf("%w (HTTP %d)", ErrRateLimited, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gh: releases request: unexpected HTTP %d", resp.StatusCode)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("gh: decode releases: %w", err)
	}
	if err := c.writeCache(releases); err != nil {
		return nil, err
	}
	// Success starts the cooldown too, so later resolves (this or another
	// process) serve the just-written cache instead of re-fetching. Written
	// after the cache so a crash can never leave cooldown-without-cache.
	_ = c.writeCooldown(c.now())
	// Mark the live fetch: a later resolve served from the cache during the
	// cooldown window is provably fresh, not the stale data the GUI's
	// stale-cache consent prompt guards against. Called under mu (releases).
	c.lastFetchAt = c.now()
	return releases, nil
}

// CacheFresh reports whether the release cache is provably fresh: this
// process fetched the release list live within the cooldown window, so
// anything served from the cache since (a concurrent or later resolve) is
// at most one cooldown old — NOT the stale cache the GUI consent prompt
// exists to guard. A cache written by an OLDER process (or only ever by a
// failed fetch) is not fresh, even inside the cooldown window.
func (c *Client) CacheFresh() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.lastFetchAt.IsZero() && c.now().Sub(c.lastFetchAt) < cooldown
}

// isRateLimited reports HTTP 429, or HTTP 403 with the rate-limit budget
// exhausted.
func isRateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode == http.StatusForbidden &&
		resp.Header.Get("X-RateLimit-Remaining") == "0"
}

// selectRelease implements requested-version semantics.
func selectRelease(releases []Release, requested string) (Release, error) {
	if requested == "latest" {
		for _, r := range releases {
			if !r.Prerelease {
				return r, nil
			}
		}
		return Release{}, fmt.Errorf("gh: requested %q: no non-prerelease release found", requested)
	}
	for _, r := range releases {
		if r.TagName == requested {
			return r, nil
		}
	}
	return Release{}, fmt.Errorf("gh: requested version %q not found in releases", requested)
}

// selectAsset applies the client's asset glob (path.Match semantics,
// case-sensitive). Multiple matches pick the lexicographically first.
func (c *Client) selectAsset(rel Release) (Asset, error) {
	var matches []Asset
	for _, a := range rel.Assets {
		if ok, err := path.Match(c.assetPattern, a.Name); err == nil && ok {
			matches = append(matches, a)
		}
	}
	if len(matches) == 0 {
		return Asset{}, fmt.Errorf("gh: release %q has no asset matching %s", rel.TagName, c.assetPattern)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
	return matches[0], nil
}

// Download streams the asset to destDir/<AssetName>, computing SHA-256
// while streaming. The write is atomic: temp file + rename. The download
// URL is resolved from the last fetch/cache load keyed by AssetName.
func (c *Client) Download(ctx context.Context, asset domain.ResolvedAsset, destDir string) (path string, sha256Hex string, err error) {
	c.mu.Lock()
	url, ok := c.downloadURLs[asset.AssetName]
	c.mu.Unlock()
	if !ok {
		return "", "", fmt.Errorf("gh: no download URL known for asset %q (resolve first)", asset.AssetName)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("gh: download %q: %w", asset.AssetName, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("gh: download %q: unexpected HTTP %d", asset.AssetName, resp.StatusCode)
	}

	tmp, err := os.CreateTemp(destDir, ".download-*")
	if err != nil {
		return "", "", err
	}
	tmpName := tmp.Name()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", "", fmt.Errorf("gh: download %q: %w", asset.AssetName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", "", err
	}

	final := filepath.Join(destDir, asset.AssetName)
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return "", "", err
	}
	return final, hex.EncodeToString(h.Sum(nil)), nil
}
