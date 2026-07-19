package discovery

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// EpicManifest is one parsed Epic Games Launcher .item manifest. The fields
// map to the launcher JSON verbatim; AppCategories is flattened to plain
// strings for classification.
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

// ParseEpicManifest parses one Epic .item manifest from r.
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

// ScanEpicManifests reads every *.item manifest in dir and returns the ones
// describing games whose install directory exists on disk. Broken manifests,
// non-games, and missing install dirs are skipped.
func ScanEpicManifests(dir string) ([]domain.Game, error) {
	items, err := filepath.Glob(filepath.Join(dir, "*.item"))
	if err != nil {
		return nil, fmt.Errorf("scan epic manifests: %w", err)
	}
	if len(items) == 0 {
		if _, statErr := os.Stat(dir); statErr != nil {
			return nil, fmt.Errorf("no epic manifest dir at %s", dir)
		}
		return nil, nil
	}
	var games []domain.Game
	for _, item := range items {
		f, err := os.Open(item)
		if err != nil {
			log.Warn().Err(err).Str("manifest", item).Msg("skipping unreadable epic manifest")
			continue
		}
		m, err := ParseEpicManifest(f)
		_ = f.Close()
		if err != nil {
			log.Warn().Err(err).Str("manifest", item).Msg("skipping broken epic manifest")
			continue
		}
		if !m.IsGame() {
			log.Debug().Str("manifest", item).Str("app", m.AppName).Msg("skipping non-game epic entry")
			continue
		}
		if st, err := os.Stat(m.InstallLocation); err != nil || !st.IsDir() {
			log.Debug().Str("manifest", item).Str("dir", m.InstallLocation).Msg("skipping epic game with missing install dir")
			continue
		}
		games = append(games, domain.Game{
			AppID:      m.AppName,
			Name:       m.DisplayName,
			InstallDir: m.InstallLocation,
			Store:      domain.StoreEpic,
			AppName:    m.AppName,
		})
	}
	return games, nil
}

// ScanEpic scans the platform's Epic Games Launcher manifest directories
// (probed per-GOOS) and returns all installed Epic games. Platforms without
// an Epic probe return nil.
func ScanEpic() []domain.Game {
	var games []domain.Game
	for _, dir := range epicManifestDirs() {
		found, err := ScanEpicManifests(dir)
		if err != nil {
			log.Debug().Err(err).Str("dir", dir).Msg("epic manifests not readable")
			continue
		}
		games = append(games, found...)
	}
	return games
}
