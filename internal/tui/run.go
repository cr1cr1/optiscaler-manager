package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Run starts the bubbletea program over sess, using the alternate screen so
// the terminal is restored on exit. version is the build version the About
// screen renders.
func Run(ctx context.Context, sess *ui.Session, version string) error {
	p := tea.NewProgram(New(sess, version), tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	return err
}
