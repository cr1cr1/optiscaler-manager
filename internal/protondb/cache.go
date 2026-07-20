package protondb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// cooldownState is the persisted cooldown file, recording the last 429/5xx
// response time.
type cooldownState struct {
	LastAttempt time.Time `json:"last_attempt"`
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

// inCooldown reports whether the last recorded 429/5xx is inside the
// cooldown window. A missing or unreadable file means no cooldown.
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

// writeCooldown records a rate-limit/server-error response time.
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

// readCache loads the persisted summary for appid.
func (c *Client) readCache(appid string) (cachedSummary, bool) {
	data, err := os.ReadFile(cacheFile(c.cacheDir, appid))
	if err != nil {
		return cachedSummary{}, false
	}
	var cs cachedSummary
	if err := json.Unmarshal(data, &cs); err != nil {
		return cachedSummary{}, false
	}
	return cs, true
}

// writeCache persists the summary for appid.
func (c *Client) writeCache(appid string, sum Summary) error {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cachedSummary{Summary: sum, FetchedAt: c.now()})
	if err != nil {
		return err
	}
	if err := os.WriteFile(cacheFile(c.cacheDir, appid), data, 0o644); err != nil {
		return fmt.Errorf("protondb: write summary cache: %w", err)
	}
	return nil
}

// writeNegative persists a 404 answer for appid.
func (c *Client) writeNegative(appid string) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(cachedSummary{FetchedAt: c.now(), NotFound: true})
	if err != nil {
		return
	}
	_ = os.WriteFile(cacheFile(c.cacheDir, appid), data, 0o644)
}
