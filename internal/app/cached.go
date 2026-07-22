package app

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/cr1cr1/optiscaler-manager/internal/version"
)

// Bundle filename shape, mirroring internal/gh/client.go's assetPrefix /
// assetSuffix: upstream embeds a date and a _MM marker, so it is never an
// exact name. Matching here (not reusing the gh constants) keeps app free
// of a gh dependency for pure filesystem work.
const (
	cachedBundlePrefix = "Optiscaler_"
	cachedBundleSuffix = ".7z"
)

// CachedVersions lists the OptiScaler bundle versions already downloaded
// under cacheDir, newest first. The version dropdown offers these so a game
// can be installed offline without a GitHub round-trip; names are returned
// verbatim (tags carry their "v" prefix) because InstallOpts.Requested
// accepts exactly those tags. Anything short of a usable bundle — a
// partial ".download-*" temp file, stray notes, a missing cache — simply
// yields no entry rather than an error: an absent cache is not a failure.
func CachedVersions(cacheDir string) []string {
	entries, err := os.ReadDir(filepath.Join(cacheDir, "optiscaler"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(
			cacheDir, "optiscaler", e.Name(),
			cachedBundlePrefix+"*"+cachedBundleSuffix))
		if err != nil {
			continue
		}
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && fi.Mode().IsRegular() {
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
