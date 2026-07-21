package ui

import (
	"context"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/protondb"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/steam"
)

// lookupBudget caps how many rows one scan enriches with live online
// requests; rows answered entirely from the clients' disk caches are free.
// The cap is a sanity bound, not a daily ration: pacing (250ms per live
// request) and the 429 cooldown are the actual rate control, so a full
// library converges inside one scan (~2 minutes worst case for ~130 rows)
// instead of over a dozen rescans. Tests pin it lower via t.Cleanup.
var lookupBudget = 128

// enrichOnline runs the "lookup" scan phase: rows with a Steam appid but a
// tail-chain title get their canonical store name first (one cheap call,
// the highest-value upgrade), then fuzzy candidates resolve titles →
// appids, and every row goes to ProtonDB. It is a no-op when lookups are
// disabled or the clients are not wired, and every per-row failure
// degrades silently. rows is the not-yet-committed scan result, mutated
// in place.
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
	// Appid-bearing tail rows first: their canonical upgrade is one
	// appdetails call each and fixes the most visible titles.
	sort.SliceStable(cands, func(a, b int) bool {
		return identifyPriority(rows[cands[a]]) > identifyPriority(rows[cands[b]])
	})
	live := 0
	done := 0
	for _, i := range cands {
		if live >= lookupBudget {
			break
		}
		if ctx.Err() != nil {
			return
		}
		identified := s.identifyRow(ctx, &rows[i], st)
		enriched := s.enrichRow(ctx, &rows[i], st, pdb)
		if identified || enriched {
			live++
		}
		done++
		s.scanProgress(phaseLookup, done, len(cands))
	}
}

// identifyPriority ranks a row for the lookup queue: rows whose appid is
// known but whose title still comes from the tail chain get the cheap,
// high-value canonical upgrade first.
func identifyPriority(r GameRow) int {
	if r.SteamAppID != "" {
		switch domain.TitleSource(r.TitleSource) {
		case domain.SourcePE, domain.SourceStem, domain.SourceFolder, "":
			return 1
		}
	}
	return 0
}

// enrichRow resolves one row's SteamAppID and ProtonTier. Both fields are
// set only on full success: a row whose appid resolved but whose summary
// failed retries via the clients' disk caches on the next scan instead of
// caching a half-enriched state. The result reports whether resolving the
// row required at least one live HTTP request — cache hits, including
// cached negative answers, do not spend the scan's lookup budget.
func (s *Session) enrichRow(ctx context.Context, row *GameRow, st *steam.Client, pdb *protondb.Client) (live bool) {
	appid := row.SteamAppID
	if appid == "" && isNumericAppID(row.AppID) {
		appid = row.AppID
	}
	if appid == "" {
		if strings.TrimSpace(row.Title) == "" {
			return false
		}
		id, _, searchLive, err := st.SearchApps(ctx, row.Title)
		live = live || searchLive
		if err != nil {
			log.Debug().Err(err).Str("title", row.Title).Msg("lookup: steam search failed")
			return live
		}
		appid = id
	}
	// The appid becomes a ProtonDB URL path segment; anything a search
	// returns that is not a bare numeric appid is rejected here.
	if !isNumericAppID(appid) {
		log.Debug().Str("appid", appid).Str("title", row.Title).Msg("lookup: skipping non-numeric appid")
		return live
	}
	sum, sumLive, err := pdb.Summary(ctx, appid)
	live = live || sumLive
	if err != nil {
		log.Debug().Err(err).Str("appid", appid).Msg("lookup: protondb summary failed")
		return live
	}
	row.SteamAppID = appid
	row.ProtonTier = sum.Tier
	return live
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
