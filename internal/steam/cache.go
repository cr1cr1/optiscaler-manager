package steam

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// cooldownState is the persisted cooldown file, recording the last 429/5xx
// response time.
type cooldownState struct {
	LastAttempt time.Time `json:"last_attempt"`
}

// cacheFile names the per-title cache file: a hash of the normalized query
// keeps titles with separators or unicode off the filesystem verbatim.
func cacheFile(cacheDir, query string) string {
	sum := sha256.Sum256([]byte(normalize(query)))
	return filepath.Join(cacheDir, "search_"+hex.EncodeToString(sum[:])[:16]+".json")
}

func normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

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

// readCache loads the persisted mapping for query.
func (c *Client) readCache(query string) (cachedSearch, bool) {
	data, err := os.ReadFile(cacheFile(c.cacheDir, query))
	if err != nil {
		return cachedSearch{}, false
	}
	var cs cachedSearch
	if err := json.Unmarshal(data, &cs); err != nil {
		return cachedSearch{}, false
	}
	return cs, true
}

// writeCache persists the resolved mapping for query.
func (c *Client) writeCache(query string, res searchResult) error {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cachedSearch{AppID: res.AppID, Name: res.Name, FetchedAt: c.now()})
	if err != nil {
		return err
	}
	if err := os.WriteFile(cacheFile(c.cacheDir, query), data, 0o644); err != nil {
		return fmt.Errorf("steam: write search cache: %w", err)
	}
	return nil
}

// writeNegative persists a no-match answer for query.
func (c *Client) writeNegative(query string) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(cachedSearch{FetchedAt: c.now(), NoMatch: true})
	if err != nil {
		return
	}
	_ = os.WriteFile(cacheFile(c.cacheDir, query), data, 0o644)
}
