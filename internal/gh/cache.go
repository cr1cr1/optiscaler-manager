package gh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// cooldownState is the persisted cooldown file. It records the last API
// attempt time; any attempt (success or failure) starts the cooldown.
type cooldownState struct {
	LastAttempt time.Time `json:"last_attempt"`
}

// inCooldown reports whether the last recorded API attempt is inside the
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

// writeCooldown records an API attempt time.
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
