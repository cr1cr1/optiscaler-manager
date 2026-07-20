package optiscalermanager

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/protondb"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/steam"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// newSession builds the interactive session both interactive frontends
// (GUI, TUI) share from command deps.
func newSession(d *Deps) *ui.Session {
	prefs, err := settings.Load(d.DataRoot)
	if err != nil {
		log.Warn().Err(err).Msg("settings unreadable, using defaults")
		prefs = settings.Defaults()
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	steamClient, protonClient := onlineClients(d.CacheDir, d.Version)
	return ui.NewSession(ui.Deps{
		Store:        d.Store,
		GH:           d.GH,
		Covers:       covers.New(httpClient, filepath.Join(d.CacheDir, "covers")),
		CacheDir:     d.CacheDir,
		Settings:     prefs,
		SettingsRoot: d.DataRoot,
		Steam:        steamClient,
		ProtonDB:     protonClient,
	})
}

// onlineClients builds the Steam/ProtonDB lookup clients that feed the
// online enrichment phase of a session scan; they share one HTTP client and
// cache under cacheDir, and report version as their user agent.
func onlineClients(cacheDir, version string) (*steam.Client, *protondb.Client) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	return steam.New(httpClient, filepath.Join(cacheDir, "steam"), version),
		protondb.New(httpClient, filepath.Join(cacheDir, "protondb"), version)
}
