package gh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// cooldownState is the persisted cooldown file. It records the last
// answered API attempt time; a rate-limited response or a success starts
// the cooldown, a transport failure does not (scope H4).
type cooldownState struct {
	// LastAnswered persists under the historical key "last_attempt" —
	// cooldown.json is read across processes, so the on-disk format stays.
	LastAnswered time.Time `json:"last_attempt"`
}

// inCooldown reports whether the last answered API call is inside the
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
	return c.now().Sub(state.LastAnswered) < cooldown
}

// writeCooldown records the time of the last answered API call (success
// or rate limit); a transport failure never reaches this.
func (c *Client) writeCooldown(t time.Time) error {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cooldownState{LastAnswered: t})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.cacheDir, cooldownFile), data, 0o644)
}

// readCache loads the persisted releases.json and indexes download URLs.
func (c *Client) readCache() ([]Release, error) {
	data, err := os.ReadFile(filepath.Join(c.cacheDir, releasesCacheFile))
	if err != nil {
		return nil, err
	}
	var releases []Release
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, fmt.Errorf("gh: decode cached releases: %w", err)
	}
	c.indexDownloadURLs(releases)
	return releases, nil
}

// writeCache persists the release list as releases.json in cacheDir.
func (c *Client) writeCache(releases []Release) error {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(releases)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.cacheDir, releasesCacheFile), data, 0o644); err != nil {
		return fmt.Errorf("gh: write releases cache: %w", err)
	}
	c.indexDownloadURLs(releases)
	return nil
}

// indexDownloadURLs records asset name → download URL for Download.
func (c *Client) indexDownloadURLs(releases []Release) {
	for _, r := range releases {
		for _, a := range r.Assets {
			c.downloadURLs[a.Name] = a.BrowserDownloadURL
		}
	}
}
