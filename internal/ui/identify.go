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
	switch domain.TitleSource(row.TitleSource) {
	case domain.SourceOverride, domain.SourceStoreID, domain.SourceGOGInfo, domain.SourceEGStore, domain.SourceUnity:
		return false
	}
	if row.SteamAppID != "" {
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
	candidates := []string{row.Title}
	if base := filepath.Base(row.InstallDir); base != row.Title {
		candidates = append(candidates, base)
	}
	for _, cand := range candidates {
		if strings.TrimSpace(cand) == "" {
			continue
		}
		items, reqLive, err := st.StoreSearch(ctx, cand)
		live = live || reqLive
		if err != nil {
			log.Debug().Err(err).Str("candidate", cand).Msg("identify: storesearch failed")
			continue
		}
		best, bestScore := -1, -1
		for i, item := range items {
			score := gid.Score(cand, item.Name, item.Windows)
			if gid.Accept(score, false) {
				row.Title = item.Name
				row.SteamAppID = item.ID
				row.TitleSource = string(domain.SourceFuzzy)
				return live
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
				return live
			}
		}
	}
	return live
}

// companyMatches compares a store developer with a PE CompanyName on
// normalized form (case, decorations, and separators ignored).
func companyMatches(developer, peCompany string) bool {
	if developer == "" || peCompany == "" {
		return false
	}
	return gid.Normalize(developer) == gid.Normalize(peCompany)
}
