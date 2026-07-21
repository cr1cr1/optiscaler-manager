package ui

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gid"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/steam"
)

// identifyRow resolves one row's canonical title in the lookup phase
// (v0.8). Manual rows only: a detected Steam appid upgrades the
// tail-chain title to the canonical store name; rows without an appid try
// the normalized fuzzy store match (candidates: current title, then the
// folder name — first acceptance wins). Override rows are frozen, store
// rows are authoritative, and in-dir metadata titles (goggame/.egstore/
// Unity) already fired their rule. live reports that at least one live
// HTTP request happened (cache hits do not spend the scan budget).
func (s *Session) identifyRow(ctx context.Context, row *GameRow, st *steam.Client) (live bool) {
	if row.Store != domain.StoreManual {
		return false
	}
	src := domain.TitleSource(row.TitleSource)
	if src == domain.SourceOverride || src == domain.SourceStoreID {
		return false // user-pinned, or already canonical
	}
	if row.SteamAppID != "" {
		if !isNumericAppID(row.SteamAppID) {
			// A hand-edited cache could carry a hostile appid; it must
			// never become a URL parameter or a cache filename.
			log.Debug().Str("appid", row.SteamAppID).Msg("identify: refusing non-numeric appid")
			return false
		}
		name, _, reqLive, err := st.AppDetails(ctx, row.SteamAppID)
		live = live || reqLive
		if err != nil {
			log.Debug().Err(err).Str("appid", row.SteamAppID).Msg("identify: appdetails failed")
			return live
		}
		if name != "" {
			row.Title = name
			row.TitleSource = string(domain.SourceStoreID)
		}
		return live
	}
	// Without an appid, in-dir metadata titles (goggame/.egstore/Unity)
	// still go through the fuzzy canonical match: a codename like Unity's
	// "STASIS2" must not stop the pipeline at a weak string.
	candidates := []string{row.Title}
	if base := filepath.Base(row.InstallDir); base != row.Title {
		candidates = append(candidates, base)
	}
	for _, cand := range candidates {
		if len(gid.Normalize(cand)) < 4 {
			// Too ambiguous to query: a codename this short can
			// exact-match an unrelated store item ("b1" → "B1").
			continue
		}
		if strings.TrimSpace(cand) == "" {
			continue
		}
		items, reqLive, err := st.StoreSearch(ctx, cand)
		live = live || reqLive
		if err != nil {
			log.Debug().Err(err).Str("candidate", cand).Msg("identify: storesearch failed")
		} else {
			// Apps outrank dlc pages; dlc is only tried when no app accepts.
			for _, types := range [2]string{"app", "dlc"} {
				accepted, reqLive := s.tryStoreItems(ctx, row, cand, items, types, st)
				live = live || reqLive
				if accepted {
					return live
				}
			}
		}
		// Steam found nothing: PCGamingWiki is the secondary canonical
		// source (GOG/off-store games).
		if s.deps.PCGW != nil {
			title, reqLive, err := s.deps.PCGW.SearchTitle(ctx, cand)
			live = live || reqLive
			if err == nil && title != "" && gid.Accept(gid.Score(cand, title, true), false) {
				row.Title = title
				row.TitleSource = string(domain.SourceFuzzy)
				return live
			}
		}
	}
	return live
}

// tryStoreItems scores the items of one store type against the candidate:
// the first outright acceptance wins, and a 75-89 best score gets one
// developer-corroboration attempt. It reports whether the row changed.
func (s *Session) tryStoreItems(ctx context.Context, row *GameRow, cand string, items []steam.StoreItem, itemType string, st *steam.Client) (accepted, live bool) {
	best, bestScore := -1, -1
	for i, item := range items {
		if item.Type != itemType {
			continue
		}
		score := gid.Score(cand, item.Name, item.Windows)
		if gid.Accept(score, false) {
			row.Title = item.Name
			row.SteamAppID = item.ID
			row.TitleSource = string(domain.SourceFuzzy)
			return true, false
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best >= 0 && bestScore >= 75 {
		_, dev, reqLive, err := st.AppDetails(ctx, items[best].ID)
		live = live || reqLive
		if err == nil && companyMatches(dev, pever.CompanyFromFile(row.ExePath)) {
			row.Title = items[best].Name
			row.SteamAppID = items[best].ID
			row.TitleSource = string(domain.SourceFuzzy)
			return true, live
		}
	}
	return false, live
}

// companyMatches compares a store developer with a PE CompanyName on
// normalized form (case, decorations, and separators ignored).
func companyMatches(developer, peCompany string) bool {
	if developer == "" || peCompany == "" {
		return false
	}
	return gid.Normalize(developer) == gid.Normalize(peCompany)
}
