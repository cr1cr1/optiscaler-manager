package optiscalermanager

import (
	"fmt"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
)

// HookCmd enables or disables the installed OptiScaler hook for a game —
// the CLI surface of the frontends' toggle. Exactly one of --enable or
// --disable is required (kong enforces the group at parse time). The
// rename is atomic and instant, so this is a synchronous core op: no
// event waiter, no --timeout.
type HookCmd struct {
	Path    string `arg:"" help:"Game root directory" type:"path"`
	Enable  bool   `help:"Enable the hook (un-park the DLL)" xor:"hookstate" required:""`
	Disable bool   `help:"Disable the hook (park the DLL)" xor:"hookstate" required:""`
}

// Run boots a one-shot session, waits for the games list, and toggles.
func (c *HookCmd) Run(d *Deps) error {
	dir := discovery.CanonicalPath(c.Path)
	sess := newSession(d)
	sess.Start(cmdContext())
	row, err := awaitRow(sess, dir, defaultOpTimeout)
	if err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	if !row.HasInstall() {
		// An uninstalled row reads Disabled=false — without this guard
		// `--enable` would report a false "already enabled" success.
		return &ExitError{Code: 1, Err: fmt.Errorf("OptiScaler is not installed for %s", c.Path)}
	}
	want := c.Disable // --disable wants Disabled=true, --enable wants false
	if row.Disabled == want {
		state := "disabled"
		if !want {
			state = "enabled"
		}
		fmt.Fprintf(d.Out, "hook already %s for %s\n", state, row.Title)
		return nil
	}
	sess.ToggleDisabled(dir)
	after, ok := rowOfSession(sess, dir)
	if !ok || after.Disabled != want {
		return &ExitError{Code: 1, Err: fmt.Errorf("hook toggle failed for %s (no hook found, or the rename failed)", c.Path)}
	}
	if want {
		fmt.Fprintf(d.Out, "disabled the OptiScaler hook for %s\n", after.Title)
	} else {
		fmt.Fprintf(d.Out, "enabled the OptiScaler hook for %s\n", after.Title)
	}
	return nil
}
