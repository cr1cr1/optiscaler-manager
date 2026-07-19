package optiscalermanager

import (
	"context"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/tui"
)

// TUICmd launches the terminal UI over the same session core as the GUI.
type TUICmd struct{}

// Run redirects logging to a file (stderr would corrupt the TUI display)
// and runs the bubbletea frontend.
func (c *TUICmd) Run(d *Deps) error {
	logPath := filepath.Join(d.DataRoot, "tui.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Logger = zerolog.Nop()
	} else {
		defer f.Close()
		log.Logger = zerolog.New(f).With().Timestamp().Logger()
	}
	return tui.Run(context.Background(), newSession(d))
}
