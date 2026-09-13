package optiscalermanager

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// SwitchCmd switches a game's OptiScaler version through the shared
// session core — the CLI surface of the GUI dropdown and the TUI `v`
// cycle. Version "latest" is re-resolved at pick time by the core (the
// v0.15 seam); an empty version means the configured default.
type SwitchCmd struct {
	Path    string        `arg:"" help:"Game root directory" type:"path"`
	Version string        `help:"Version tag to install, or 'latest' (re-resolved at pick time; default: the configured default version)"`
	Timeout time.Duration `help:"Max wait for the operation"`
}

// Run boots a one-shot session, waits for the games list to settle,
// dispatches the switch, and waits for the chain's single settle event.
// Consent gates are answered on the terminal (or declined when stdin is
// not interactive); a busy target op fails fast. The path is
// canonicalized (rows store canonical install dirs).
func (c *SwitchCmd) Run(d *Deps) error {
	dir := discovery.CanonicalPath(c.Path)
	sess := newSession(d)
	sess.Start(cmdContext())
	if _, err := awaitRow(sess, dir, opTimeout(c.Timeout)); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	version := c.Version
	if version == "" {
		version = sess.Settings().DefaultVersion
	}
	sess.SwitchVersion(dir, version)
	ev, err := waitForOpKinds(sess, dir, opTimeout(c.Timeout), d.ErrOut, ui.EvOpSettled)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	after, _ := rowOfSession(sess, dir)
	switch {
	case strings.HasPrefix(ev.Text, "switched to"):
		fmt.Fprintf(d.Out, "switched %s → %s\n", after.Title, after.OptiScalerVersion)
	case strings.HasPrefix(ev.Text, "already at"):
		fmt.Fprintf(d.Out, "%s (%s)\n", ev.Text, after.Title)
	default:
		// "switch failed: …", "switch cancelled", "cannot resolve …",
		// "unknown game dir", "game is not installed; nothing to switch"
		// — fail loud with the reason.
		return &ExitError{Code: 1, Err: errors.New(ev.Text)}
	}
	return nil
}
