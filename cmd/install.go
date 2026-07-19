package optiscalermanager

import (
	"context"
	"errors"
	"fmt"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
)

// InstallCmd installs the current OptiScaler bundle into a game directory.
type InstallCmd struct {
	Path        string `arg:"" help:"Game root directory" type:"path"`
	Force       bool   `help:"Install even into EAC-protected games (ban risk)"`
	AllowCached bool   `help:"Accept cached release info when the GitHub API is rate-limited"`
}

// Run resolves the bundle, downloads it, and runs the transaction.
func (c *InstallCmd) Run(d *Deps) error {
	m, err := app.Install(context.Background(), d.Store, d.GH, d.CacheDir, c.Path, app.InstallOpts{
		AllowCached: c.AllowCached,
		EACOverride: c.Force,
	})
	if errors.Is(err, app.ErrEACProtected) {
		return fmt.Errorf("%v (use --force to proceed anyway)", err)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Out, "installed %s (%s) into %s\n", m.Resolved.AssetName, m.Resolved.Version, m.InstallDir)
	return nil
}

// UninstallCmd reverses a committed install.
type UninstallCmd struct {
	Path string `arg:"" help:"Game root directory" type:"path"`
}

// Run resolves the install dir and uninstalls.
func (c *UninstallCmd) Run(d *Deps) error {
	dir, err := app.Uninstall(context.Background(), d.Store, c.Path)
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Out, "uninstalled from %s\n", dir)
	return nil
}

// RollbackCmd restores a game after an interrupted or failed install.
type RollbackCmd struct {
	Path string `arg:"" help:"Game root directory" type:"path"`
}

// Run resolves the install dir and rolls back.
func (c *RollbackCmd) Run(d *Deps) error {
	dir, err := app.Rollback(context.Background(), d.Store, c.Path)
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Out, "rolled back %s\n", dir)
	return nil
}
