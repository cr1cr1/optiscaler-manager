package optiscalermanager

import (
	"errors"
	"fmt"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// DLSSUpdateCmd updates the game's existing three-file NVIDIA runtime set
// (DLSS, DLSSD, DLSS-G) through the shared session core — cache-first,
// never adding a missing DLL.
type DLSSUpdateCmd struct {
	Path    string        `arg:"" help:"Game root directory" type:"path"`
	Timeout time.Duration `help:"Max wait for the operation"`
}

// Run boots a one-shot session, waits for the games list to settle,
// dispatches the update, and waits for the outcome.
func (c *DLSSUpdateCmd) Run(d *Deps) error {
	dir := discovery.CanonicalPath(c.Path)
	sess := newSession(d)
	sess.Start(cmdContext())
	if _, err := awaitRow(sess, dir, opTimeout(c.Timeout)); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	sess.UpdateDLSS(dir)
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

// DLSSRestoreCmd restores a previously snapshotted NVIDIA runtime set; an
// empty Snapshot id means the newest backup. The restore runs behind the
// session's consent gate — answered on the terminal, declined when stdin
// is not interactive (no --yes exists).
type DLSSRestoreCmd struct {
	Path     string        `arg:"" help:"Game root directory" type:"path"`
	Snapshot string        `help:"Snapshot id (default: newest)"`
	Timeout  time.Duration `help:"Max wait for the operation"`
}

// Run boots a one-shot session, waits for the games list, validates the
// snapshot id against the existing backups (an unknown id would otherwise
// silently stage nothing), dispatches the restore, and waits for the
// outcome.
func (c *DLSSRestoreCmd) Run(d *Deps) error {
	dir := discovery.CanonicalPath(c.Path)
	sess := newSession(d)
	sess.Start(cmdContext())
	if _, err := awaitRow(sess, dir, opTimeout(c.Timeout)); err != nil {
		return &ExitError{Code: 1, Err: err}
	}
	snaps := sess.DLSSSnapshots(dir)
	id := c.Snapshot
	if id == "" {
		if len(snaps) == 0 {
			return &ExitError{Code: 1, Err: fmt.Errorf("no DLSS snapshots for %s; nothing to restore", dir)}
		}
		id = snaps[0].ID
	} else {
		known := false
		for _, s := range snaps {
			if s.ID == id {
				known = true
				break
			}
		}
		if !known {
			return &ExitError{Code: 1, Err: fmt.Errorf("unknown DLSS snapshot id %s for %s", id, dir)}
		}
	}
	sess.RestoreDLSS(dir, id)
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
