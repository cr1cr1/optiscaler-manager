package optiscalermanager

import (
	"context"
	"fmt"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/installer"
)

// InstallCmd installs the current OptiScaler bundle into a game directory.
type InstallCmd struct {
	Path        string `arg:"" help:"Game root directory" type:"path"`
	Force       bool   `help:"Install even into EAC-protected games (ban risk)"`
	AllowCached bool   `help:"Accept cached release info when the GitHub API is rate-limited"`
}

// Run resolves the bundle, downloads it, and runs the transaction.
func (c *InstallCmd) Run(d *Deps) error {
	ctx := context.Background()

	gameRoot, err := canonicalDir(c.Path)
	if err != nil {
		return err
	}
	installDir, err := discovery.ResolveInstallDir(gameRoot)
	if err != nil {
		return err
	}
	if installer.EACProtected(gameRoot) && !c.Force {
		return fmt.Errorf("%s is EAC-protected; installing OptiScaler risks a ban (use --force to proceed anyway)", gameRoot)
	}

	resolved, fromCache, err := d.GH.Resolve(ctx, "latest")
	if err != nil {
		return err
	}
	if fromCache && !c.AllowCached {
		return fmt.Errorf("GitHub API rate-limited; release info is stale cache (use --allow-cached to proceed with it)")
	}
	if fromCache {
		fmt.Fprintf(d.ErrOut, "warning: using cached release info (rate limit cooldown)\n")
	}

	bundlePath, digest, err := d.GH.Download(ctx, resolved, d.CacheDir)
	if err != nil {
		return err
	}
	resolved.SHA256 = digest

	m, err := installer.Install(ctx, d.Store, installer.Request{
		GameRoot:         gameRoot,
		InstallDir:       installDir,
		ArchivePath:      bundlePath,
		RequestedVersion: "latest",
		Resolved:         resolved,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Out, "installed %s (%s) into %s\n", resolved.AssetName, resolved.Version, m.InstallDir)
	return nil
}

// UninstallCmd reverses a committed install.
type UninstallCmd struct {
	Path string `arg:"" help:"Game root directory" type:"path"`
}

// Run resolves the install dir and uninstalls.
func (c *UninstallCmd) Run(d *Deps) error {
	id, installDir, err := manifestIDForGame(c.Path)
	if err != nil {
		return err
	}
	if err := installer.Uninstall(context.Background(), d.Store, id); err != nil {
		return err
	}
	fmt.Fprintf(d.Out, "uninstalled from %s\n", installDir)
	return nil
}

// RollbackCmd restores a game after an interrupted or failed install.
type RollbackCmd struct {
	Path string `arg:"" help:"Game root directory" type:"path"`
}

// Run resolves the install dir and rolls back.
func (c *RollbackCmd) Run(d *Deps) error {
	id, installDir, err := manifestIDForGame(c.Path)
	if err != nil {
		return err
	}
	if err := installer.Rollback(context.Background(), d.Store, id); err != nil {
		return err
	}
	fmt.Fprintf(d.Out, "rolled back %s\n", installDir)
	return nil
}

// manifestIDForGame maps a game root to its manifest ID via the resolved
// install directory.
func manifestIDForGame(gameRoot string) (id, installDir string, err error) {
	root, err := canonicalDir(gameRoot)
	if err != nil {
		return "", "", err
	}
	dir, err := discovery.ResolveInstallDir(root)
	if err != nil {
		return "", "", err
	}
	id, err = installer.ManifestIDFor(dir)
	if err != nil {
		return "", "", err
	}
	return id, dir, nil
}
