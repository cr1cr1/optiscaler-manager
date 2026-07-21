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

// DefaultLaunchTemplate runs the game exe with its args, nothing else; it
// mirrors the launch package's built-in manual template.
const DefaultLaunchTemplate = `"{exe}" {args}`

// Settings are the user-configurable preferences.
type Settings struct {
	DefaultVersion string   `json:"default_version"`
	LaunchTemplate string   `json:"launch_template"`
	ExtraDirs      []string `json:"extra_dirs,omitempty"`
	// OnlineLookups gates ProtonDB/Steam game-info enrichment during
	// scans. It defaults to true: the bool zero value is false, so Load
	// decodes through a pointer to tell "missing key" (legacy file →
	// true) apart from an explicit false.
	OnlineLookups bool `json:"online_lookups"`
	// TitleOverrides pins display titles per canonical install dir; an
	// override beats every identification rule (v0.8). JSON-edited only
	// for now.
	TitleOverrides map[string]string `json:"title_overrides,omitempty"`
}

// Defaults returns the out-of-box settings.
func Defaults() Settings {
	return Settings{DefaultVersion: "latest", LaunchTemplate: DefaultLaunchTemplate, OnlineLookups: true}
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
	// Decode through a pointer for OnlineLookups so a legacy file without
	// the key defaults to true while an explicit false stays false.
	var raw struct {
		DefaultVersion string            `json:"default_version"`
		LaunchTemplate string            `json:"launch_template"`
		ExtraDirs      []string          `json:"extra_dirs,omitempty"`
		OnlineLookups  *bool             `json:"online_lookups"`
		TitleOverrides map[string]string `json:"title_overrides,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Defaults(), fmt.Errorf("settings: parse: %w", err)
	}
	s := Settings{
		DefaultVersion: raw.DefaultVersion,
		LaunchTemplate: raw.LaunchTemplate,
		ExtraDirs:      raw.ExtraDirs,
		OnlineLookups:  true,
		TitleOverrides: raw.TitleOverrides,
	}
	if raw.OnlineLookups != nil {
		s.OnlineLookups = *raw.OnlineLookups
	}
	if s.DefaultVersion == "" {
		s.DefaultVersion = "latest"
	}
	if s.LaunchTemplate == "" {
		s.LaunchTemplate = DefaultLaunchTemplate
	}
	return s, nil
}

// Save persists settings atomically (temp file + rename). An empty root is a
// no-op: sessions without a state dir must not fail or spam callers.
func Save(root string, s Settings) error {
	if root == "" {
		return nil
	}
	if s.DefaultVersion == "" {
		s.DefaultVersion = "latest"
	}
	if s.LaunchTemplate == "" {
		s.LaunchTemplate = DefaultLaunchTemplate
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
