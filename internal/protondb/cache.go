package protondb

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/jsoncache"
)

// cachedSummary is the persisted summary with its fetch time. NotFound
// marks a negative entry: ProtonDB answered 404 for the appid, a stable
// answer cached for the same TTL so unknown appids are not re-fetched
// every scan.
type cachedSummary struct {
	Summary   Summary   `json:"summary"`
	FetchedAt time.Time `json:"fetched_at"`
	NotFound  bool      `json:"not_found,omitempty"`
}

// cacheFile names the per-appid cache file.
func cacheFile(cacheDir, appid string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, appid)
	return filepath.Join(cacheDir, "summary_"+safe+".json")
}

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

// readCache loads the persisted summary for appid.
func (c *Client) readCache(appid string) (cachedSummary, bool) {
	return jsoncache.Read[cachedSummary](cacheFile(c.cacheDir, appid))
}

// writeCache persists the summary for appid.
func (c *Client) writeCache(appid string, sum Summary) error {
	cs := cachedSummary{Summary: sum, FetchedAt: c.now()}
	if err := jsoncache.Write(cacheFile(c.cacheDir, appid), cs); err != nil {
		return fmt.Errorf("protondb: write summary cache: %w", err)
	}
	return nil
}

// writeNegative persists a 404 answer for appid.
func (c *Client) writeNegative(appid string) {
	_ = jsoncache.Write(cacheFile(c.cacheDir, appid), cachedSummary{FetchedAt: c.now(), NotFound: true})
}
