package optiscalermanager

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// ScanCmd lists locally installed games (all supported stores) with detected
// upscalers and, for managed installs, per-game version info.
type ScanCmd struct {
	SteamRoot string `help:"Scan this Steam root instead of auto-detecting all of them" type:"path" env:"OM_STEAM_ROOT"`
}

// Run executes the scan.
func (c *ScanCmd) Run(d *Deps) error {
	s, _ := settings.Load(d.DataRoot) // unreadable settings → Defaults
	entries, err := app.ScanAllLibraries(context.Background(), d.Store, app.ScanAllOptions{
		SteamRoot: c.SteamRoot,
		ExtraDirs: s.ExtraDirs,
		Resolver: discovery.ChainResolver(func(dir string) string {
			return s.TitleOverrides[discovery.CanonicalPath(dir)]
		}),
	})
	if err != nil {
		return err
	}
	for _, e := range entries {
		tech := "-"
		if len(e.Tech) > 0 {
			tech = strings.Join(e.Tech, ",")
		}
		marker := ""
		if e.EAC {
			marker = " [EAC]"
		}
		status := ""
		if e.Status != "" {
			status = fmt.Sprintf(" [%s]", e.Status)
		}
		fmt.Fprintf(d.Out, "%s\t%s\t%s\t%s\t%s\t%s%s%s\n",
			e.Game.Name, e.Game.AppID, e.Game.Store.String(), tech, versionsColumn(e), e.Game.InstallDir, marker, status)
	}
	return nil
}

// versionsColumn renders "OptiScaler 0.9.4, DLSS 3.7.10, FSR 3.1.4" for
// managed installs, "-" otherwise. Component keys sort for stable output.
func versionsColumn(e app.LibraryEntry) string {
	parts := []string{}
	if e.OptiScalerVersion != "" {
		parts = append(parts, "OptiScaler "+e.OptiScalerVersion)
	}
	keys := make([]string, 0, len(e.ComponentVersions))
	for k := range e.ComponentVersions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, e.ComponentVersions[k])
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}
