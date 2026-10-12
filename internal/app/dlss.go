package app

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
// DLSS directory (ResolveDLSSDir). It never installs a missing runtime DLL.
// Downloads are cached per commit under cacheRoot; commitHint, when its
// cache dir is complete (the startup check's published commit already
// fetched), installs from the cache with zero network. Rollback backups
// live beside the DLLs they protect (dlss-backups/); a pending backup
// over MaxBackupNoConfirm refuses with *dlss.LargeBackupError unless
// allowLarge consents.
func UpdateDLSS(ctx context.Context, client *dlss.Client, cacheRoot, gameRoot, commitHint string, allowLarge bool) (dlss.Snapshot, error) {
	dir, err := ResolveDLSSDir(gameRoot)
	if err != nil {
		return dlss.Snapshot{}, err
	}
	return dlss.Update(ctx, client, cacheRoot, dir, commitHint, MaxBackupNoConfirm, allowLarge)
}

// RestoreDLSS restores a complete prior NVIDIA runtime set into a game's
// DLSS directory, under the same large-backup consent gate as UpdateDLSS.
func RestoreDLSS(ctx context.Context, gameRoot, snapshotID string, allowLarge bool) (dlss.Snapshot, error) {
	dir, err := ResolveDLSSDir(gameRoot)
	if err != nil {
		return dlss.Snapshot{}, err
	}
	return dlss.Restore(ctx, dir, snapshotID, MaxBackupNoConfirm, allowLarge)
}

// DLSSSnapshots lists complete prior NVIDIA runtime sets for a game.
func DLSSSnapshots(gameRoot string) ([]dlss.Snapshot, error) {
	dir, err := ResolveDLSSDir(gameRoot)
	if err != nil {
		return nil, err
	}
	return dlss.Snapshots(dir)
}

// dlssAnchor is OptiScaler's frame-generation DLL: wherever it lives is
// where the game loads its NVIDIA runtime set from, so it is where DLSS
// updates install (issue 081).
const dlssAnchor = "dlssg_to_fsr3_amd_is_better.dll"

// ResolveDLSSDir returns the directory a game's NVIDIA runtime set lives
// in: the directory holding OptiScaler's dlssg_to_fsr3_amd_is_better.dll
// when OptiScaler is installed (the OptiScaler/ subdir for the dlssnr
// fork, the injection dir itself for standard layouts), else the resolved
// injection dir. Backup stores are never candidates: an uninstalled
// OptiScaler's relocated copy must not hijack the target.
func ResolveDLSSDir(gameRoot string) (string, error) {
	root, err := canonicalDir(gameRoot)
	if err != nil {
		return "", err
	}
	if anchor, err := findAnchor(root); err != nil {
		return "", err
	} else if anchor != "" {
		return anchor, nil
	}
	return discovery.ResolveInstallDir(root)
}

// DLSSPresenceDir returns where the game's present NVIDIA runtime lives:
// the anchor dir first when it holds any runtime member, else the
// injection dir when it holds any, else "". OptiScaler is NOT a
// precondition — the pill reflects whatever set is present, OptiScaler
// dir first, then the bin dir (issue 082). The updater still installs
// into ResolveDLSSDir; this describes what is there, not what the
// updater manages.
func DLSSPresenceDir(gameRoot, injectionDir string) string {
	if root, err := canonicalDir(gameRoot); err == nil {
		if anchor, _ := findAnchor(root); anchor != "" && hasRuntimeMember(anchor) {
			return anchor
		}
	}
	if injectionDir != "" && hasRuntimeMember(injectionDir) {
		return injectionDir
	}
	return ""
}

// findAnchor returns the directory holding OptiScaler's
// dlssg_to_fsr3_amd_is_better.dll ("" when none): the OptiScaler/ subdir
// for the dlssnr fork, the injection dir itself for standard layouts.
// Backup stores are never candidates: an uninstalled OptiScaler's
// relocated copy must not hijack the target.
func findAnchor(root string) (string, error) {
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Same noise dirs as classify.DirFiles: the rollback stores
			// hold relocated copies, never the live set.
			switch d.Name() {
			case ".git", "dlss-backups", "optiscaler-backups":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(d.Name(), dlssAnchor) {
			hits = append(hits, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(hits) > 0 {
		// Several anchors should not happen; the smallest path wins so
		// the choice stays deterministic.
		sort.Strings(hits)
		return hits[0], nil
	}
	return "", nil
}

// hasRuntimeMember reports whether dir holds any member of the NVIDIA
// runtime set as a top-level regular file.
func hasRuntimeMember(dir string) bool {
	for _, name := range dlss.Files {
		if st, err := os.Stat(filepath.Join(dir, name)); err == nil && st.Mode().IsRegular() {
			return true
		}
	}
	return false
}
