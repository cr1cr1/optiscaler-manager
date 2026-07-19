package optiscalermanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
)

// ScanCmd lists locally installed Steam games with detected upscalers.
type ScanCmd struct {
	SteamRoot string `help:"Scan this Steam root instead of auto-detecting all of them" type:"path" env:"OM_STEAM_ROOT"`
}

// Run executes the scan.
func (c *ScanCmd) Run(d *Deps) error {
	entries, err := app.ScanLibrary(context.Background(), d.Store, c.SteamRoot)
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
		fmt.Fprintf(d.Out, "%s\t%s\t%s\t%s%s%s\n", e.Game.Name, e.Game.AppID, tech, e.Game.InstallDir, marker, status)
	}
	return nil
}
