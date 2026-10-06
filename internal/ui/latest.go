package ui

import (
	"context"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// startupPreload is the one-shot startup version check and pre-warm (user
// spec: "at program startup, optiscaler and nvidia DLSS dlls versions are
// checked, and `latest` downloaded in the cache directory"): resolve the
// latest OptiScaler release once, memoize its tag (the version dropdown
// renders it as the "Latest" option), and download its bundle into the
// download cache when it is not already cached. The DLSS half rides in
// CheckDLSS's tags goroutine (one tags call, then a Preload of the
// published set). Failures degrade silently: the menu falls back to the
// concrete cached versions and the next press resolves online as before.
func (s *Session) startupPreload(ctx context.Context) {
	client := s.ghClient()
	if client == nil {
		return
	}
	if !s.Settings().OnlineLookups {
		return
	}
	resolved, _, err := client.Resolve(ctx, "latest")
	if err != nil {
		log.Debug().Err(err).Msg("startup latest check failed; no Latest option this boot")
		return
	}
	s.setLatestTag(resolved.Version)
	fork := s.Settings().Active()
	bundleRoot := settings.BundleCacheDir(s.deps.CacheDir, fork.Slug)
	for _, v := range app.CachedVersions(bundleRoot, fork.AssetPattern) {
		if v == resolved.Version {
			return
		}
	}
	dir := filepath.Join(bundleRoot, resolved.Version)
	if _, _, err := client.Download(ctx, resolved, dir); err != nil {
		log.Warn().Err(err).Str("tag", resolved.Version).Msg("startup latest bundle preload failed")
		return
	}
	log.Info().Str("tag", resolved.Version).Msg("startup preloaded the latest OptiScaler bundle")
}

// setLatestTag records the startup-resolved latest tag under the session
// lock.
func (s *Session) setLatestTag(tag string) {
	s.mu.Lock()
	s.latestTag = tag
	s.mu.Unlock()
}

// LatestKnown reports the concrete tag the startup latest check resolved
// ("" when unknown: offline boot, resolution failed, or no check ran).
// The version dropdown renders it as the "Latest (…)" option; it never
// resolves here — the check runs once at program start.
func (s *Session) LatestKnown() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latestTag
}
