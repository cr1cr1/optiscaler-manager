package optiscalermanager

import (
	"errors"
	"fmt"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// LaunchCmd requests a fire-and-forget game launch through the shared
// session core — the same deep-link path the GUI and TUI use. A successful
// request proves nothing about the game actually running; the report says
// "requested", never "launched".
type LaunchCmd struct {
	Path    string        `arg:"" help:"Game root directory" type:"path"`
	Timeout time.Duration `help:"Max wait for the launch request"`
}

// Run boots a one-shot session, waits for the games list to settle,
// dispatches the launch, and waits for the request outcome.
func (c *LaunchCmd) Run(d *Deps) error {
	dir := discovery.CanonicalPath(c.Path)
	sess := newSession(d)
	sess.Start(cmdContext())
	if _, err := awaitRow(sess, dir, opTimeout(c.Timeout)); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	sess.Launch(dir)
	ev, err := waitForOp(sess, dir, opTimeout(c.Timeout), d.ErrOut)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if ev.Kind != ui.EvOpDone {
		// Failed or cancelled — the event text carries the reason.
		return &ExitError{Code: 1, Err: errors.New(ev.Text)}
	}
	fmt.Fprintf(d.Out, "%s\n", ev.Text)
	return nil
}
