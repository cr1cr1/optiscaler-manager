package optiscalermanager

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/classify"
	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/installer"
)

// ScanCmd lists locally installed Steam games with detected upscalers.
type ScanCmd struct {
	SteamRoot string `help:"Scan this Steam root instead of auto-detecting all of them" type:"path" env:"OM_STEAM_ROOT"`
}

// Run executes the scan.
func (c *ScanCmd) Run(d *Deps) error {
	roots := []string{}
	if c.SteamRoot != "" {
		roots = append(roots, c.SteamRoot)
	} else {
		roots = discovery.SteamRoots()
	}
	if len(roots) == 0 {
		return fmt.Errorf("no Steam installation found (set OM_STEAM_ROOT or use --steam-root)")
	}

	seen := map[string]bool{}
	for _, root := range roots {
		games, err := discovery.ScanSteam(root)
		if err != nil {
			log.Warn().Err(err).Str("root", root).Msg("scan failed for root")
			fmt.Fprintf(d.ErrOut, "warning: %s: %v\n", root, err)
			continue
		}
		for _, g := range games {
			if seen[g.InstallDir] {
				continue
			}
			seen[g.InstallDir] = true
			printGame(d, g)
		}
	}
	return nil
}

func printGame(d *Deps, g domain.Game) {
	kinds := map[string]bool{}
	for _, c := range classify.Dir(g.InstallDir) {
		kinds[c.Kind.String()] = true
	}
	tech := "-"
	if len(kinds) > 0 {
		var list []string
		for k := range kinds {
			list = append(list, k)
		}
		tech = strings.Join(list, ",")
	}
	marker := ""
	if installer.EACProtected(g.InstallDir) {
		marker = " [EAC]"
	}
	fmt.Fprintf(d.Out, "%s\t%s\t%s\t%s%s\n", g.Name, g.AppID, tech, g.InstallDir, marker)
}
