package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Run starts the bubbletea program over sess, using the alternate screen so
// the terminal is restored on exit.
func Run(ctx context.Context, sess *ui.Session) error {
	p := tea.NewProgram(New(sess), tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	return err
}
