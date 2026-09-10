package app

import (
	"context"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
)

// UpdateDLSS replaces the complete existing NVIDIA runtime set in a game's
// resolved injection directory. It never installs a missing runtime DLL.
// Downloads are cached per commit under cacheRoot.
func UpdateDLSS(ctx context.Context, client *dlss.Client, cacheRoot, dataRoot, gameRoot string) (dlss.Snapshot, error) {
	dir, err := resolveInjectionDir(gameRoot)
	if err != nil {
		return dlss.Snapshot{}, err
	}
	return dlss.Update(ctx, client, cacheRoot, dataRoot, dir)
}

// RestoreDLSS restores a complete prior NVIDIA runtime set into a game's
// resolved injection directory.
func RestoreDLSS(ctx context.Context, dataRoot, gameRoot, snapshotID string) (dlss.Snapshot, error) {
	dir, err := resolveInjectionDir(gameRoot)
	if err != nil {
		return dlss.Snapshot{}, err
	}
	return dlss.Restore(ctx, dataRoot, dir, snapshotID)
}

// DLSSSnapshots lists complete prior NVIDIA runtime sets for a game.
func DLSSSnapshots(dataRoot, gameRoot string) ([]dlss.Snapshot, error) {
	dir, err := resolveInjectionDir(gameRoot)
	if err != nil {
		return nil, err
	}
	return dlss.Snapshots(dataRoot, dir)
}

// resolveInjectionDir canonicalizes a game root and resolves its injection
// directory — the shared prelude of the DLSS operations (same semantics as
// Install's: canonical dir, ResolveInstallDir).
func resolveInjectionDir(gameRoot string) (string, error) {
	root, err := canonicalDir(gameRoot)
	if err != nil {
		return "", err
	}
	dir, err := discovery.ResolveInstallDir(root)
	if err != nil {
		return "", err
	}
	return dir, nil
}
