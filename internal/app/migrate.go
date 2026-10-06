package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// MigrateLegacyBundleCache moves the pre-fork cache layout — releases.json
// and cooldown.json at the cache root, and optiscaler/<tag>/ bundle dirs —
// into the upstream fork's namespace (settings.BundleCacheDir). Fork
// namespaces (directories containing the "__" fork-key separator) are left
// alone. Best-effort by design: a cache is regenerable, so a failed or
// colliding move is skipped, never destructively merged.
func MigrateLegacyBundleCache(cacheDir string) {
	upstream := settings.BundleCacheDir(cacheDir, settings.DefaultForkSlug)

	move := func(src, dst string) {
		if err := os.Rename(src, dst); err != nil {
			log.Debug().Err(err).Str("src", src).Str("dst", dst).
				Msg("legacy cache move skipped")
		}
	}

	for _, f := range []string{"releases.json", "cooldown.json"} {
		src := filepath.Join(cacheDir, f)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.MkdirAll(upstream, 0o755); err != nil {
			return
		}
		move(src, filepath.Join(upstream, f))
	}

	entries, err := os.ReadDir(filepath.Join(cacheDir, "optiscaler"))
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || strings.Contains(e.Name(), "__") {
			continue
		}
		if err := os.MkdirAll(upstream, 0o755); err != nil {
			return
		}
		move(filepath.Join(cacheDir, "optiscaler", e.Name()),
			filepath.Join(upstream, e.Name()))
	}
}
