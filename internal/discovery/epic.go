package discovery

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gid"
)

// Epic manifest type and parser moved to internal/gid (v0.8); the aliases
// keep this package's API stable while gid stays below discovery.
type EpicManifest = gid.EpicManifest

var ParseEpicManifest = gid.ParseEpicManifest

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
