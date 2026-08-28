// Package jsoncache is the shared read/write helper for the small JSON
// state files the API clients persist between runs: per-query result
// caches and rate-limit cooldown markers. A missing or corrupt file reads
// as a cache miss, never an error.
package jsoncache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Read loads the JSON value at path. The bool reports a hit; a missing or
// undecodable file is a miss.
func Read[T any](path string) (T, bool) {
	var v T
	data, err := os.ReadFile(path)
	if err != nil {
		return v, false
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return v, false
	}
	return v, true
}

// Write persists v as JSON at path, creating the parent directory.
func Write[T any](path string, v T) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// cooldownState is the persisted cooldown file, recording the last 429/5xx
// response time.
type cooldownState struct {
	LastAttempt time.Time `json:"last_attempt"`
}

// InCooldown reports whether the last recorded 429/5xx is inside the
// window before now. A missing or unreadable file means no cooldown.
func InCooldown(path string, now time.Time, window time.Duration) bool {
	state, ok := Read[cooldownState](path)
	if !ok {
		return false
	}
	return now.Sub(state.LastAttempt) < window
}

// WriteCooldown records a rate-limit/server-error response time.
func WriteCooldown(path string, t time.Time) error {
	return Write(path, cooldownState{LastAttempt: t})
}
