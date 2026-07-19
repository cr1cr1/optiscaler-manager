package discovery

import (
	"os"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// registryReader is the seam between GOG/Steam discovery logic and the
// Windows registry. Paths are registry key paths relative to
// HKEY_LOCAL_MACHINE (backslash-separated). The production implementation
// lives in registry_windows.go; tests drive discovery with a fake.
type registryReader interface {
	// Subkeys returns the names of the immediate subkeys under path.
	Subkeys(path string) ([]string, error)
	// ReadString returns the string value name stored under path.
	ReadString(path, name string) (string, error)
}

// gogGamesFromRegistry enumerates GOG games from the registry key base: each
// subkey is one game carrying gameName, path, and gameID values. Entries
// with missing values or missing install directories are skipped. Resolved
// games are enriched with their primary executable from goggame-*.info.
func gogGamesFromRegistry(reg registryReader, base string) []domain.Game {
	ids, err := reg.Subkeys(base)
	if err != nil {
		log.Debug().Err(err).Str("key", base).Msg("no GOG registry key")
		return nil
	}
	var games []domain.Game
	for _, id := range ids {
		key := base + `\` + id
		name, _ := reg.ReadString(key, "gameName")
		path, _ := reg.ReadString(key, "path")
		gameID, _ := reg.ReadString(key, "gameID")
		if gameID == "" {
			gameID = id
		}
		if name == "" || path == "" {
			log.Debug().Str("key", key).Msg("skipping GOG entry with missing values")
			continue
		}
		if st, err := os.Stat(path); err != nil || !st.IsDir() {
			log.Debug().Str("key", key).Str("path", path).Msg("skipping GOG game with missing install dir")
			continue
		}
		games = append(games, domain.Game{
			AppID:      gameID,
			Name:       name,
			InstallDir: path,
			Store:      domain.StoreGOG,
			ExePath:    GOGExePath(path),
		})
	}
	return games
}
