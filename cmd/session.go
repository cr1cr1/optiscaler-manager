package optiscalermanager

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
	"github.com/cr1cr1/optiscaler-manager/internal/pcgw"
	"github.com/cr1cr1/optiscaler-manager/internal/protondb"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/steam"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// cmdContext is the one-shot command context: no deadline of its own — the
// per-command --timeout bounds the wait, the op context bounds the work.
func cmdContext() context.Context { return context.Background() }

// rowOfSession returns the snapshot row for dir (found=false when the dir
// is unknown to the session).
func rowOfSession(sess *ui.Session, dir string) (ui.GameRow, bool) {
	rows := sess.Snapshot().Rows
	for i := range rows {
		if rows[i].InstallDir == dir {
			return rows[i], true
		}
	}
	return ui.GameRow{}, false
}

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
	pcgwClient := pcgw.New(httpClient, filepath.Join(d.CacheDir, "pcgw"), d.Version)
	dlssClient := d.DLSS
	if dlssClient == nil {
		dlssClient = dlss.New(httpClient)
	}
	coverClient := d.Covers
	if coverClient == nil {
		coverClient = covers.New(httpClient, filepath.Join(d.CacheDir, "covers"))
		coverClient.PCGW = pcgwClient
		coverClient.UserAgent = "optiscaler-manager/" + d.Version + " (https://github.com/cr1cr1/optiscaler-manager)"
	}
	sess := ui.NewSession(ui.Deps{
		Store:        d.Store,
		GH:           d.GH,
		NewGH:        d.NewGH,
		DLSS:         dlssClient,
		Covers:       coverClient,
		CacheDir:     d.CacheDir,
		Settings:     prefs,
		SettingsRoot: d.DataRoot,
		SteamRoot:    d.SteamRoot,
		Steam:        steamClient,
		ProtonDB:     protonClient,
		PCGW:         pcgwClient,
		Launcher:     d.Launcher, // nil selects the platform default
		UmuLauncher:  newUmuLauncher(prefs),
	})
	return sess
}

// onlineClients builds the Steam/ProtonDB lookup clients that feed the
// online enrichment phase of a session scan; they share one HTTP client and
// cache under cacheDir, and report version as their user agent.
func onlineClients(cacheDir, version string) (*steam.Client, *protondb.Client) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	return steam.New(httpClient, filepath.Join(cacheDir, "steam"), version),
		protondb.New(httpClient, filepath.Join(cacheDir, "protondb"), version)
}
