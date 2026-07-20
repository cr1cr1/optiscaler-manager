package optiscalermanager

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
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
	return ui.NewSession(ui.Deps{
		Store:        d.Store,
		GH:           d.GH,
		Covers:       covers.New(&http.Client{Timeout: 10 * time.Second}, filepath.Join(d.CacheDir, "covers")),
		CacheDir:     d.CacheDir,
		Settings:     prefs,
		SettingsRoot: d.DataRoot,
	})
}
