//go:build linux

package discovery

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gid"
)

// heroicConfigDirs returns the Heroic Games Launcher config roots to
// probe: the native install ($XDG_CONFIG_HOME/heroic, falling back to
// ~/.config/heroic) and the Flatpak one. Roots that don't exist are
// skipped by the file probe itself.
func heroicConfigDirs() []string {
	var dirs []string
	home, homeErr := os.UserHomeDir()
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		dirs = append(dirs, filepath.Join(xdg, "heroic"))
	} else if homeErr == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "heroic"))
	}
	if homeErr == nil {
		dirs = append(dirs, filepath.Join(home, ".var", "app",
			"com.heroicgameslauncher.hgl", "config", "heroic"))
	}
	return dirs
}

// heroicGames discovers Epic and GOG games installed through Heroic from
// its installed.json records. Games whose install directory is missing,
// non-Windows builds, and broken records are skipped; overlaps with other
// sources are resolved by ScanAll's canonical-dir dedupe.
func heroicGames() []domain.Game {
	var games []domain.Game
	for _, cfg := range heroicConfigDirs() {
		epic := filepath.Join(cfg, "legendaryConfig", "legendary", "installed.json")
		games = append(games, heroicStoreGames(epic, domain.StoreEpic)...)
		gog := filepath.Join(cfg, "gog_store", "installed.json")
		games = append(games, heroicStoreGames(gog, domain.StoreGOG)...)
	}
	return games
}

// heroicStoreGames parses one installed.json and maps its entries to
// games of the given store. A missing file is normal (store unused);
// a broken file is warned about and skipped.
func heroicStoreGames(path string, store domain.Store) []domain.Game {
	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Debug().Err(err).Str("file", path).Msg("heroic installed.json unreadable")
		}
		return nil
	}
	defer func() { _ = f.Close() }()
	entries, err := gid.ParseHeroicInstalled(f)
	if err != nil {
		log.Warn().Err(err).Str("file", path).Msg("skipping broken heroic installed.json")
		return nil
	}
	var games []domain.Game
	for _, e := range entries {
		if g, ok := heroicEntryGame(e, store); ok {
			games = append(games, g)
		}
	}
	return games
}

// heroicEntryGame maps one Heroic record to a game. Non-Windows builds and
// records whose install directory no longer exists are dropped.
func heroicEntryGame(e gid.HeroicEntry, store domain.Store) (domain.Game, bool) {
	if !e.IsWindows() {
		log.Debug().Str("app", e.AppName).Str("platform", e.Platform).
			Msg("skipping non-windows heroic entry")
		return domain.Game{}, false
	}
	st, err := os.Stat(e.InstallPath)
	if err != nil || !st.IsDir() {
		log.Debug().Str("app", e.AppName).Str("dir", e.InstallPath).
			Msg("skipping heroic game with missing install dir")
		return domain.Game{}, false
	}
	g := domain.Game{
		AppID:      e.AppName,
		Name:       e.Title,
		InstallDir: e.InstallPath,
		Store:      store,
		ExePath:    heroicExe(e, store),
	}
	if store == domain.StoreEpic {
		g.AppName = e.AppName
	}
	return g, true
}

// heroicExe resolves the record's executable (relative to the install dir,
// possibly with windows separators) to an existing on-disk path. GOG
// entries without a recorded executable fall back to the goggame-*.info
// play tasks Heroic's GOG installs ship. "" when nothing resolves.
func heroicExe(e gid.HeroicEntry, store domain.Store) string {
	rel := strings.ReplaceAll(e.Executable, `\`, string(filepath.Separator))
	if rel != "" {
		if p, ok := joinWithin(e.InstallPath, rel); ok {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
			log.Debug().Str("app", e.AppName).Str("exe", e.Executable).
				Msg("heroic executable not on disk")
		} else {
			log.Warn().Str("app", e.AppName).Str("exe", e.Executable).
				Msg("heroic executable escapes install dir, rejected")
		}
	}
	if store == domain.StoreGOG {
		return GOGExePath(e.InstallPath)
	}
	return ""
}
