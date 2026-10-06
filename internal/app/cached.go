package app

import (
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/cr1cr1/optiscaler-manager/internal/version"
)

// CachedVersions lists the OptiScaler bundle versions already downloaded
// under bundleDir (the per-fork cache namespace, <tag>/ subdirs), newest
// first. assetPattern is the fork's release-asset glob (path.Match
// semantics): a <tag> dir counts only when it holds a regular file whose
// base name matches. The version dropdown offers these so a game can be
// installed offline without a GitHub round-trip; names are returned
// verbatim (tags carry their "v" prefix) because InstallOpts.Requested
// accepts exactly those tags. Anything short of a usable bundle — a
// partial ".download-*" temp file, stray notes, a missing cache — simply
// yields no entry rather than an error: an absent cache is not a failure.
func CachedVersions(bundleDir, assetPattern string) []string {
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(bundleDir, e.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if !f.Type().IsRegular() {
				continue
			}
			if ok, _ := path.Match(assetPattern, f.Name()); ok {
				out = append(out, e.Name())
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return version.Compare(out[i], out[j]) > 0
	})
	return out
}
