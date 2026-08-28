package steam

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/jsoncache"
)

// cachedSearch is the persisted title → appid mapping with its fetch
// time. NoMatch marks a negative entry: the search yielded no plausible
// match, a stable answer cached for the same TTL so unresolvable titles
// are not re-fetched every scan.
type cachedSearch struct {
	AppID     string    `json:"appid"`
	Name      string    `json:"name"`
	FetchedAt time.Time `json:"fetched_at"`
	NoMatch   bool      `json:"no_match,omitempty"`
}

// cacheFile names the per-title cache file: a hash of the normalized query
// keeps titles with separators or unicode off the filesystem verbatim.
func cacheFile(cacheDir, query string) string {
	sum := sha256.Sum256([]byte(normalize(query)))
	return filepath.Join(cacheDir, "search_"+hex.EncodeToString(sum[:])[:16]+".json")
}

func normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func (c *Client) cooldownPath() string { return filepath.Join(c.cacheDir, cooldownFile) }

// inCooldown reports whether the last recorded 429/5xx is inside the
// cooldown window. A missing or unreadable file means no cooldown.
func (c *Client) inCooldown() bool {
	return jsoncache.InCooldown(c.cooldownPath(), c.now(), cooldown)
}

// writeCooldown records a rate-limit/server-error response time.
func (c *Client) writeCooldown(t time.Time) error {
	return jsoncache.WriteCooldown(c.cooldownPath(), t)
}

// readCache loads the persisted mapping for query.
func (c *Client) readCache(query string) (cachedSearch, bool) {
	return jsoncache.Read[cachedSearch](cacheFile(c.cacheDir, query))
}

// writeCache persists the resolved mapping for query.
func (c *Client) writeCache(query string, res searchResult) error {
	cs := cachedSearch{AppID: res.AppID, Name: res.Name, FetchedAt: c.now()}
	if err := jsoncache.Write(cacheFile(c.cacheDir, query), cs); err != nil {
		return fmt.Errorf("steam: write search cache: %w", err)
	}
	return nil
}

// writeNegative persists a no-match answer for query.
func (c *Client) writeNegative(query string) {
	_ = jsoncache.Write(cacheFile(c.cacheDir, query), cachedSearch{FetchedAt: c.now(), NoMatch: true})
}
