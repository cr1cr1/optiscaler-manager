// Package settings persists user preferences as a small JSON document in
// the data root.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Settings are the user-configurable preferences.
type Settings struct {
	DefaultVersion string   `json:"default_version"`
	ExtraDirs      []string `json:"extra_dirs,omitempty"`
}

// Defaults returns the out-of-box settings.
func Defaults() Settings {
	return Settings{DefaultVersion: "latest"}
}

func path(root string) string { return filepath.Join(root, "settings.json") }

// Load reads settings from root, returning Defaults when none exist.
func Load(root string) (Settings, error) {
	data, err := os.ReadFile(path(root))
	if errors.Is(err, fs.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return Defaults(), fmt.Errorf("settings: read: %w", err)
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Defaults(), fmt.Errorf("settings: parse: %w", err)
	}
	if s.DefaultVersion == "" {
		s.DefaultVersion = "latest"
	}
	return s, nil
}

// Save persists settings atomically (temp file + rename).
func Save(root string, s Settings) error {
	if s.DefaultVersion == "" {
		s.DefaultVersion = "latest"
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(root, ".settings-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path(root))
}
