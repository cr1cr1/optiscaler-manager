package ui

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// cacheSchemaVersion is the on-disk schema of the games-list cache. Bump on
// any breaking change to the cached row shape.
const cacheSchemaVersion = 1

// gamesCache is the persisted games list: display-ready rows written on
// every library change and read once at Start so a warm boot skips the scan.
type gamesCache struct {
	Version int       `json:"version"`
	Rows    []GameRow `json:"rows"`
}

func gamesCachePath(root string) string { return filepath.Join(root, "games.json") }

// loadGamesCache reads the cached rows. A missing, unreadable, corrupt, or
// stale-schema cache yields nil — never an error — so callers fall through
// to a real scan.
func loadGamesCache(root string) []GameRow {
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
	return c.Rows
}

// saveGamesCache persists rows atomically (temp file + rename, mirroring
// settings.Save). Best-effort: a failed write only forfeits a warm next
// start, so it logs and never fails the caller.
func saveGamesCache(root string, rows []GameRow) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		log.Warn().Err(err).Msg("games cache: create root")
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
