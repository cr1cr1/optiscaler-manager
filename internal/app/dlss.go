package app

import (
	"context"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
)

// UpdateDLSS replaces the complete existing NVIDIA runtime set in a game's
// resolved injection directory. It never installs a missing runtime DLL.
func UpdateDLSS(ctx context.Context, client *dlss.Client, dataRoot, gameRoot string) (dlss.Snapshot, error) {
	root, err := canonicalDir(gameRoot)
	if err != nil {
		return dlss.Snapshot{}, err
	}
	dir, err := discovery.ResolveInstallDir(root)
	if err != nil {
		return dlss.Snapshot{}, err
	}
	return dlss.Update(ctx, client, dataRoot, dir)
}

// RestoreDLSS restores a complete prior NVIDIA runtime set into a game's
// resolved injection directory.
func RestoreDLSS(ctx context.Context, dataRoot, gameRoot, snapshotID string) (dlss.Snapshot, error) {
	root, err := canonicalDir(gameRoot)
	if err != nil {
		return dlss.Snapshot{}, err
	}
	dir, err := discovery.ResolveInstallDir(root)
	if err != nil {
		return dlss.Snapshot{}, err
	}
	return dlss.Restore(ctx, dataRoot, dir, snapshotID)
}

// DLSSSnapshots lists complete prior NVIDIA runtime sets for a game.
func DLSSSnapshots(dataRoot, gameRoot string) ([]dlss.Snapshot, error) {
	root, err := canonicalDir(gameRoot)
	if err != nil {
		return nil, err
	}
	dir, err := discovery.ResolveInstallDir(root)
	if err != nil {
		return nil, err
	}
	return dlss.Snapshots(dataRoot, dir)
}
