package gui

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/rs/zerolog/log"
	shireiapp "go.hasen.dev/shirei/app"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// renderToPNG is the headless render seam used by the smoke test.
var renderToPNG = RenderToPNG

// Run opens the window and drives the frame loop until it closes.
func Run(ctx context.Context, cfg Config) error {
	shireiapp.SetupWindow("optiscaler-manager", 1100, 700)
	m := newModel(cfg)
	go m.scan(ctx)
	shireiapp.Run(m.rootView)
	return nil
}

// scan loads the library off the frame goroutine.
func (m *model) scan(ctx context.Context) {
	entries, err := app.ScanLibrary(ctx, m.cfg.Store, m.cfg.SteamRoot)
	WithFrameLock(func() {
		if err != nil {
			m.status = "Scan failed: " + err.Error()
			log.Warn().Err(err).Msg("library scan failed")
			return
		}
		m.rows = entries
		sortRows(m.rows)
		m.status = fmt.Sprintf("%d games", len(m.rows))
	})
	RequestNextFrame()
}

// markStatus updates one row's install status after an operation.
func (m *model) markStatus(gameRoot string, s domain.Status) {
	for i := range m.rows {
		if m.rows[i].Game.InstallDir == gameRoot {
			m.rows[i].Status = s
			sortRows(m.rows)
			return
		}
	}
}

func (m *model) install(gameRoot string, eacOK bool) {
	WithFrameLock(func() { m.busy = true; m.status = "Installing…" })
	res, err := app.Install(context.Background(), m.cfg.Store, m.cfg.GH, m.cfg.CacheDir, gameRoot,
		app.InstallOpts{EACOverride: eacOK})
	WithFrameLock(func() {
		m.busy = false
		if err != nil {
			m.status = "Install failed: " + err.Error()
			log.Error().Err(err).Str("game", gameRoot).Msg("install failed")
			return
		}
		m.status = "Installed " + res.Resolved.Version
		m.markStatus(res.GameRoot, domain.StatusCommitted)
	})
	RequestNextFrame()
}

func (m *model) uninstall(gameRoot string) {
	WithFrameLock(func() { m.busy = true; m.status = "Uninstalling…" })
	dir, err := app.Uninstall(context.Background(), m.cfg.Store, gameRoot)
	WithFrameLock(func() {
		m.busy = false
		if err != nil {
			m.status = "Uninstall failed: " + err.Error()
			log.Error().Err(err).Str("game", gameRoot).Msg("uninstall failed")
			return
		}
		m.status = "Uninstalled from " + dir
		m.markStatus(gameRoot, "")
	})
	RequestNextFrame()
}

func (m *model) rollback(gameRoot string) {
	WithFrameLock(func() { m.busy = true; m.status = "Rolling back…" })
	dir, err := app.Rollback(context.Background(), m.cfg.Store, gameRoot)
	WithFrameLock(func() {
		m.busy = false
		if err != nil {
			m.status = "Rollback failed: " + err.Error()
			log.Error().Err(err).Str("game", gameRoot).Msg("rollback failed")
			return
		}
		m.status = "Rolled back " + dir
		m.markStatus(gameRoot, domain.StatusRolledBack)
	})
	RequestNextFrame()
}

func (m *model) openEditor(path string) {
	if err := exec.Command("xdg-open", path).Start(); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("open editor failed")
		WithFrameLock(func() { m.status = "Cannot open editor: " + err.Error() })
		RequestNextFrame()
	}
}
