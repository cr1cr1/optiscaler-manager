package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// resolvedDefaultFreshWindow mirrors the gh client's 15-minute rate-limit
// cooldown: inside it, a Resolve the session performed live is exactly
// what the release cache serves, so letting an install reuse that cache
// without the stale-cache consent prompt is safe (the prompt exists for
// genuinely stale caches, not for data fetched moments ago).
const resolvedDefaultFreshWindow = 15 * time.Minute

// errInstallPaused marks an install that stopped at a consent gate (EAC
// warning or stale-cache question); AnswerConfirm owns the continuation.
// It never reaches the user.
var errInstallPaused = errors.New("ui: install paused for consent")

// errOpBusy marks an op that never started because the game already has
// one in flight; the caller only learns it could not proceed.
var errOpBusy = errors.New("ui: operation already in progress")

// resolvedDefault returns the memoized concrete tag the configured default
// version resolved to, or "" when unknown: never resolved, resolution
// failed (offline), or DefaultVersion changed since — a memo is only valid
// for the exact configured value it was resolved from, so a stale tag can
// never be served for a new setting.
func (s *Session) resolvedDefault() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resolvedDefaultKey != s.deps.Settings.DefaultVersion {
		return ""
	}
	return s.resolvedDefaultVersion
}

// refreshResolvedDefault resolves the configured default version to a
// concrete release tag exactly once per distinct value: the memo is keyed
// by the requested value, so later scans (and every row/frame within them)
// reuse it without touching the network, and a settings change re-resolves
// once on the next scan. Failures are NOT memoized — the next scan
// retries — and leave the memo keyed to the old value, which
// resolvedDefault refuses to serve for the new one (safe offline
// degradation: the memo stays empty for the new value).
func (s *Session) refreshResolvedDefault(ctx context.Context, requested string) {
	if requested == "" {
		return
	}
	s.mu.Lock()
	memoized := s.resolvedDefaultKey
	memoizedAt := s.resolvedDefaultAt
	s.mu.Unlock()
	// A zero timestamp marks a provisional offline memo (pinned tag served
	// without validation): it never blocks a real resolution.
	if memoized == requested && !memoizedAt.IsZero() {
		return
	}
	resolve := s.resolveVersion
	if resolve == nil {
		resolve = s.ghResolveVersion
	}
	resolved, fresh, err := resolve(ctx, requested)
	if err != nil {
		log.Debug().Err(err).Str("requested", requested).
			Msg("default version resolution failed; memo left empty this scan")
		return
	}
	s.mu.Lock()
	s.resolvedDefaultKey = requested
	s.resolvedDefaultVersion = resolved
	s.resolvedDefaultFresh = fresh
	s.resolvedDefaultAt = s.now()
	s.mu.Unlock()
}

// warmBootResolveDefault populates the resolved-default memo after a warm
// boot from the games cache, exactly like a scan would (online: one
// bounded resolve; offline: pinned tags only) — the version dropdown
// needs the memo before the next manual scan.
func (s *Session) warmBootResolveDefault(ctx context.Context) {
	snap := s.Settings()
	if snap.OnlineLookups {
		s.refreshResolvedDefault(ctx, snap.DefaultVersion)
	} else {
		s.memoizePinnedDefault(snap.DefaultVersion)
	}
}

// memoizePinnedDefault serves a pinned concrete default without the
// network: an exact tag needs no resolution (gh would only exact-match
// it), so with online lookups off the resolved default is still known.
// "latest" is unresolvable offline — no memo. The memo is
// provisional (zero timestamp): refreshResolvedDefault re-resolves it
// for real once lookups are back on, and defaultRecentlyResolved stays
// false so installs keep their stale-cache consent semantics.
func (s *Session) memoizePinnedDefault(requested string) {
	if requested == "" || requested == "latest" {
		return
	}
	s.mu.Lock()
	s.resolvedDefaultKey = requested
	s.resolvedDefaultVersion = requested
	s.resolvedDefaultFresh = false
	s.resolvedDefaultAt = time.Time{}
	s.mu.Unlock()
}

// ghResolveVersion is the production resolve seam: one bounded gh.Resolve.
// fresh reports that the answer came from a live fetch, not the cache.
func (s *Session) ghResolveVersion(ctx context.Context, requested string) (string, bool, error) {
	if s.deps.GH == nil {
		return "", false, fmt.Errorf("ui: no GitHub client configured")
	}
	resolved, fromCache, err := s.deps.GH.Resolve(ctx, requested)
	if err != nil {
		return "", false, err
	}
	return resolved.Version, !fromCache, nil
}

// defaultRecentlyResolved reports whether the release cache is provably
// fresh because THIS session fetched it live within the gh cooldown
// window. Installs may then serve fromCache without the stale-cache
// consent prompt — the prompt's "stale" premise does not hold.
func (s *Session) defaultRecentlyResolved() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resolvedDefaultFresh &&
		!s.resolvedDefaultAt.IsZero() &&
		s.now().Sub(s.resolvedDefaultAt) < resolvedDefaultFreshWindow
}
