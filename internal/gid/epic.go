package gid

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// EpicManifest is one parsed Epic Games Launcher manifest (.item or an
// in-dir .egstore/*.manifest). The fields map to the launcher JSON
// verbatim; AppCategories is flattened to plain strings for classification.
type EpicManifest struct {
	AppName         string
	DisplayName     string
	InstallLocation string
	AppCategories   []string
}

// IsGame reports whether the manifest describes a game (as opposed to a
// plugin, addon, or tool the launcher also tracks).
func (m EpicManifest) IsGame() bool {
	for _, c := range m.AppCategories {
		if strings.Contains(strings.ToLower(c), "games") {
			return true
		}
	}
	return false
}

type epicManifestJSON struct {
	AppName         string `json:"AppName"`
	DisplayName     string `json:"DisplayName"`
	InstallLocation string `json:"InstallLocation"`
	AppCategories   []struct {
		Category string `json:"Category"`
		Path     string `json:"Path"`
	} `json:"AppCategories"`
}

// ParseEpicManifest parses one Epic manifest from r.
func ParseEpicManifest(r io.Reader) (EpicManifest, error) {
	var raw epicManifestJSON
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return EpicManifest{}, fmt.Errorf("epic manifest: %w", err)
	}
	if raw.AppName == "" || raw.InstallLocation == "" {
		return EpicManifest{}, fmt.Errorf("epic manifest: missing required field (AppName=%q InstallLocation=%q)", raw.AppName, raw.InstallLocation)
	}
	m := EpicManifest{
		AppName:         raw.AppName,
		DisplayName:     raw.DisplayName,
		InstallLocation: raw.InstallLocation,
	}
	for _, c := range raw.AppCategories {
		if c.Category != "" {
			m.AppCategories = append(m.AppCategories, c.Category)
		}
		if c.Path != "" {
			m.AppCategories = append(m.AppCategories, c.Path)
		}
	}
	if m.DisplayName == "" {
		m.DisplayName = raw.AppName
	}
	return m, nil
}
