package discovery

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// ScanOptions controls ScanAll. A nil SteamRoots slice means "probe the
// platform's Steam roots"; RecursiveRoots lists manually managed roots whose
// subdirectories are individual games. Progress, when non-nil, is called
// after each probed root with the number of roots done and the total root
// count (steam + recursive). Resolver, when non-nil, is the identification
// chain for manual rows (default ChainResolver without overrides).
type ScanOptions struct {
	SteamRoots     []string
	RecursiveRoots []string
	Progress       func(done, total int)
	Resolver       TitleResolver
}

// ScanAll discovers games from every store the platform supports — Steam,
// Epic, GOG, .app bundles (macOS), and manual recursive roots — and merges
// them into one list deduplicated by canonical install directory. When the
// same directory appears under several stores, the earlier store in probe
// order (Steam, Epic, GOG, apps, manual) wins.
func ScanAll(ctx context.Context, opts ScanOptions) ([]domain.Game, error) {
	var games []domain.Game
	seen := map[string]bool{}
	add := func(found []domain.Game) {
		for _, g := range found {
			key := canonicalPath(g.InstallDir)
			if seen[key] {
				continue
			}
			seen[key] = true
			g.InstallDir = key
			games = append(games, g)
		}
	}

	steamRoots := opts.SteamRoots
	if steamRoots == nil {
		steamRoots = SteamRoots()
	}
	done := 0
	total := len(steamRoots) + len(opts.RecursiveRoots)
	tick := func() {
		done++
		if opts.Progress != nil {
			opts.Progress(done, total)
		}
	}
	for _, root := range steamRoots {
		if err := ctx.Err(); err != nil {
			return games, err
		}
		found, err := ScanSteam(root)
		if err != nil {
			log.Debug().Err(err).Str("root", root).Msg("steam root not scannable")
			tick()
			continue
		}
		add(found)
		tick()
	}
	if err := ctx.Err(); err != nil {
		return games, err
	}
	add(ScanEpic())
	add(gogGames())
	add(storeApps())
	resolver := opts.Resolver
	if resolver == nil {
		resolver = ChainResolver(nil)
	}
	for _, root := range opts.RecursiveRoots {
		if err := ctx.Err(); err != nil {
			return games, err
		}
		found, err := ScanRecursiveWithResolver(ctx, root, resolver)
		if err != nil {
			log.Debug().Err(err).Str("root", root).Msg("manual root not scannable")
			tick()
			continue
		}
		add(found)
		tick()
	}
	return games, nil
}
