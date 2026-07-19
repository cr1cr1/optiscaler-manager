package optiscalermanager

import (
	"context"

	"github.com/cr1cr1/optiscaler-manager/internal/gui"
)

// GUICmd launches the graphical interface. It is the default command when
// optiscaler-manager is invoked without arguments.
type GUICmd struct {
	AuditGrid bool `help:"Show the raw audit grid instead of the action list"`
}

// Run builds the interactive session and opens the window.
func (c *GUICmd) Run(d *Deps) error {
	return gui.Run(context.Background(), gui.Config{Session: newSession(d), AuditGrid: c.AuditGrid, Version: d.Version})
}
