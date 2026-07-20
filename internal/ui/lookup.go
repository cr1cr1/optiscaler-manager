package ui

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/protondb"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/steam"
)

// lookupBudget caps how many rows one scan enriches online; the rest are
// skipped silently and retried by a later scan. Combined with the clients'
// 250ms request pacing this bounds the phase to a few seconds.
const lookupBudget = 8

// enrichOnline runs the "lookup" scan phase: manual rows without a Steam
// appid resolve title → appid via Steam search, rows with a numeric appid
// (steam-library games) go straight to ProtonDB, and every success sets
// SteamAppID/ProtonTier on the row. It is a no-op when lookups are disabled
// or the clients are not wired, and every per-row failure degrades
// silently. rows is the not-yet-committed scan result, mutated in place.
func (s *Session) enrichOnline(ctx context.Context, rows []GameRow, snap settings.Settings) {
	if !snap.OnlineLookups {
		return
	}
	st, pdb := s.deps.Steam, s.deps.ProtonDB
	if st == nil || pdb == nil {
		return
	}
	var cands []int
	for i := range rows {
		if rows[i].ProtonTier != "" {
			continue
		}
		if rows[i].SteamAppID != "" || isNumericAppID(rows[i].AppID) || rows[i].Store == domain.StoreManual {
			cands = append(cands, i)
		}
	}
	total := min(len(cands), lookupBudget)
	for n, i := range cands {
		if n >= lookupBudget {
			break
		}
		if ctx.Err() != nil {
			return
		}
		s.enrichRow(ctx, &rows[i], st, pdb)
		s.scanProgress(phaseLookup, n+1, total)
	}
}

// enrichRow resolves one row's SteamAppID and ProtonTier. Both fields are
// set only on full success: a row whose appid resolved but whose summary
// failed retries via the clients' disk caches on the next scan instead of
// caching a half-enriched state.
func (s *Session) enrichRow(ctx context.Context, row *GameRow, st *steam.Client, pdb *protondb.Client) {
	appid := row.SteamAppID
	if appid == "" && isNumericAppID(row.AppID) {
		appid = row.AppID
	}
	if appid == "" {
		if strings.TrimSpace(row.Title) == "" {
			return
		}
		id, _, err := st.SearchApps(ctx, row.Title)
		if err != nil {
			log.Debug().Err(err).Str("title", row.Title).Msg("lookup: steam search failed")
			return
		}
		appid = id
	}
	sum, _, err := pdb.Summary(ctx, appid)
	if err != nil {
		log.Debug().Err(err).Str("appid", appid).Msg("lookup: protondb summary failed")
		return
	}
	row.SteamAppID = appid
	row.ProtonTier = sum.Tier
}

// isNumericAppID reports whether id is a bare Steam appid (all digits), as
// opposed to a "manual_"/"custom_" manual id or an Epic/GOG identifier.
func isNumericAppID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
