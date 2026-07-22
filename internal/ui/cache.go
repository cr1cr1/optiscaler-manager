package ui

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rs/zerolog/log"
)

// cacheSchemaVersion is the on-disk schema of the games-list cache. Bump on
// any breaking change to the cached row shape or semantics (2: v0.7 made
// container scan-roots row-less, so v0.6 caches carry stale self-rows; 3:
// v0.7.1 made containers transparent at every level, so v0.7 caches carry
// phantom container rows; 4: v0.7.2 removed platform/junk rows and changed
// title resolution, so v0.7.1 caches carry rejected rows; 5: v0.8 added
// identification sources and Steam app ids to rows, so v0.7.2 caches
// carry titles the new resolver would replace).
const cacheSchemaVersion = 5

// gamesCache is the persisted games list: display-ready rows written on
// every library change and read once at Start so a warm boot skips the scan.
type gamesCache struct {
	Version int       `json:"version"`
	Rows    []GameRow `json:"rows"`
}

func gamesCachePath(root string) string { return filepath.Join(root, "games.json") }

// loadGamesCache reads the cached rows. A missing, unreadable, corrupt, or
// stale-schema cache yields nil — never an error — so callers fall through
// to a real scan. goos follows the launch.New idiom ("" = runtime.GOOS):
// proton tiers are linux-only, so off-linux loads strip them in place (no
// schema bump — the next save self-heals the file). Upgrade offers are
// stripped on EVERY platform: they were computed from the resolved default
// version at save time and can go stale while the app is closed (a changed
// DefaultVersion or a newer release retargets them), and a warm boot
// rehydrates rows without revalidation — the next scan recomputes
// eligibility from a fresh memo.
func loadGamesCache(root, goos string) []GameRow {
	if goos == "" {
		goos = runtime.GOOS
	}
	data, err := os.ReadFile(gamesCachePath(root))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		log.Warn().Err(err).Msg("games cache unreadable, rescanning")
		return nil
	}
	var c gamesCache
	if err := json.Unmarshal(data, &c); err != nil {
		log.Warn().Err(err).Msg("games cache corrupt, rescanning")
		return nil
	}
	if c.Version != cacheSchemaVersion {
		return nil
	}
	for i := range c.Rows {
		c.Rows[i].UpgradeAvailable = false
		c.Rows[i].UpgradeTarget = ""
	}
	if goos != "linux" {
		for i := range c.Rows {
			c.Rows[i].ProtonTier = ""
		}
	}
	return c.Rows
}

// saveGamesCache persists rows atomically (temp file + rename, mirroring
// settings.Save). Best-effort: a failed write only forfeits a warm next
// start, so it logs and never fails the caller. A missing root is skipped,
// never recreated: the root is created at session construction, and a
// vanished root means the session is being torn down.
func saveGamesCache(root string, rows []GameRow) {
	if _, err := os.Stat(root); err != nil {
		log.Debug().Str("root", root).Msg("games cache: root gone, skipping write")
		return
	}
	data, err := json.Marshal(gamesCache{Version: cacheSchemaVersion, Rows: rows})
	if err != nil {
		log.Warn().Err(err).Msg("games cache: marshal")
		return
	}
	tmp, err := os.CreateTemp(root, ".games-*.json")
	if err != nil {
		log.Warn().Err(err).Msg("games cache: temp file")
		return
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		log.Warn().Err(err).Msg("games cache: write")
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		log.Warn().Err(err).Msg("games cache: close")
		return
	}
	if err := os.Rename(tmp.Name(), gamesCachePath(root)); err != nil {
		_ = os.Remove(tmp.Name())
		log.Warn().Err(err).Msg("games cache: rename")
	}
}
