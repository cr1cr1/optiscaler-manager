package app

import (
	"context"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
)

// MaxBackupNoConfirm is the consent budget (issue 023): an operation
// whose pending backup exceeds this many bytes pauses for explicit user
// confirmation instead of writing. A var so tests can shrink it. THE
// policy owner — installer.defaultMaxBackupNoConfirm mirrors it for
// direct library callers.
var MaxBackupNoConfirm = int64(100 << 20)

// UpdateDLSS replaces the complete existing NVIDIA runtime set in a game's
// resolved injection directory. It never installs a missing runtime DLL.
// Downloads are cached per commit under cacheRoot; commitHint, when its
// cache dir is complete (the startup check's published commit already
// fetched), installs from the cache with zero network. Rollback backups
// live in the game directory itself (dlss-backups/); a pending backup
// over MaxBackupNoConfirm refuses with *dlss.LargeBackupError unless
// allowLarge consents.
func UpdateDLSS(ctx context.Context, client *dlss.Client, cacheRoot, gameRoot, commitHint string, allowLarge bool) (dlss.Snapshot, error) {
	dir, err := resolveInjectionDir(gameRoot)
	if err != nil {
		return dlss.Snapshot{}, err
	}
	return dlss.Update(ctx, client, cacheRoot, dir, commitHint, MaxBackupNoConfirm, allowLarge)
}

// RestoreDLSS restores a complete prior NVIDIA runtime set into a game's
// resolved injection directory, under the same large-backup consent gate
// as UpdateDLSS.
func RestoreDLSS(ctx context.Context, gameRoot, snapshotID string, allowLarge bool) (dlss.Snapshot, error) {
	dir, err := resolveInjectionDir(gameRoot)
	if err != nil {
		return dlss.Snapshot{}, err
	}
	return dlss.Restore(ctx, dir, snapshotID, MaxBackupNoConfirm, allowLarge)
}

// DLSSSnapshots lists complete prior NVIDIA runtime sets for a game.
func DLSSSnapshots(gameRoot string) ([]dlss.Snapshot, error) {
	dir, err := resolveInjectionDir(gameRoot)
	if err != nil {
		return nil, err
	}
	return dlss.Snapshots(dir)
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
