package ui

import (
	"context"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// UmuGameIDFor resolves the GAMEID/STORE pair for a umu launch (issue
// 064) so protonfixes can apply per-game fixes:
//
//  1. a per-game override (GameIDOverrides, JSON-edited) wins;
//  2. an identified game (SteamAppID known) is umu-<appid> — the
//     umu-database ID rule: a game on Steam gets umu-<SteamAppID> even
//     when owned on another store, so Valve's appid-keyed fixes and
//     protonfixes both apply;
//  3. Heroic titles fall back to a umu-database lookup by
//     (store, codename) — Epic AppName / GOG product id;
//  4. anything else is "umu-default" with an empty store (no fixes,
//     all Proton optimizations).
//
// Lookup failures degrade along the chain — a launch is never blocked
// by the database.
func (s *Session) UmuGameIDFor(ctx context.Context, row GameRow) (gameID, store string) {
	store = umuStoreID(row.Store)
	if id := s.Settings().GameIDOverrides[canonicalDir(row.InstallDir)]; id != "" {
		return id, store
	}
	if row.SteamAppID != "" {
		return "umu-" + row.SteamAppID, store
	}
	codename := ""
	switch row.Store {
	case domain.StoreEpic, domain.StoreGOG:
		codename = row.AppID
	}
	if store != "" && codename != "" {
		s.mu.Lock()
		fn := s.deps.UmuGameIDLookup
		s.mu.Unlock()
		if fn != nil {
			if id, err := fn(ctx, store, codename); err == nil && id != "" {
				return id, store
			}
		}
	}
	return "umu-default", store
}

// umuStoreID maps a domain store to the umu-database store id; manual
// games have none ("" — protonfixes then searches its umu gamefixes
// directory).
func umuStoreID(st domain.Store) string {
	switch st {
	case domain.StoreEpic:
		return "egs"
	case domain.StoreGOG:
		return "gog"
	case domain.StoreSteam:
		return "steam"
	}
	return ""
}
