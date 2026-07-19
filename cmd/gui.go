package optiscalermanager

import (
	"context"
	"path/filepath"

	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/gui"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// GUICmd launches the graphical interface. It is the default command when
// optiscaler-manager is invoked without arguments.
type GUICmd struct {
	AuditGrid bool `help:"Show the raw audit grid instead of the action list"`
}

// Run builds the interactive session and opens the window.
func (c *GUICmd) Run(d *Deps) error {
	sess := ui.NewSession(ui.Deps{
		Store:    d.Store,
		GH:       d.GH,
		Covers:   covers.New(nil, filepath.Join(filepath.Dir(d.CacheDir), "covers")),
		CacheDir: d.CacheDir,
	})
	return gui.Run(context.Background(), gui.Config{Session: sess, AuditGrid: c.AuditGrid})
}
