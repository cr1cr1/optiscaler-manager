package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/version"
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
// for the exact configured value it was resolved from, so a stale target
// can never be offered for a new setting.
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
// degradation: no target, no offer).
func (s *Session) refreshResolvedDefault(ctx context.Context, requested string) {
	if requested == "" {
		return
	}
	s.mu.Lock()
	memoized := s.resolvedDefaultKey
	s.mu.Unlock()
	if memoized == requested {
		return
	}
	resolve := s.resolveVersion
	if resolve == nil {
		resolve = s.ghResolveVersion
	}
	resolved, fresh, err := resolve(ctx, requested)
	if err != nil {
		log.Debug().Err(err).Str("requested", requested).
			Msg("default version resolution failed; upgrade offers suppressed this scan")
		return
	}
	s.mu.Lock()
	s.resolvedDefaultKey = requested
	s.resolvedDefaultVersion = resolved
	s.resolvedDefaultFresh = fresh
	s.resolvedDefaultAt = s.now()
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

// upgradeOffer computes a row's upgrade eligibility: a committed or
// external install whose known version is older than the resolved default
// gets the offer; everything else (unmanaged, failed/in-progress, unknown
// installed version, unknown default, up-to-date) gets none.
func upgradeOffer(status domain.Status, installed, resolvedDefault string) (bool, string) {
	if installed == "" || resolvedDefault == "" {
		return false, ""
	}
	if status != domain.StatusCommitted && status != domain.StatusExternal {
		return false, ""
	}
	if version.Compare(installed, resolvedDefault) >= 0 {
		return false, ""
	}
	return true, resolvedDefault
}

// Upgrade starts an upgrade of an eligible row. Committed rows chain
// uninstall-then-install; external rows adopt-install directly. Ineligible
// rows are a silent no-op (the offer raced a state change).
func (s *Session) Upgrade(gameDir string) {
	go s.doUpgrade(gameDir)
}

func (s *Session) doUpgrade(gameDir string) {
	row := s.findRow(gameDir)
	if row == nil || !row.UpgradeAvailable || row.UpgradeTarget == "" {
		return
	}
	log.Info().Str("gameDir", gameDir).
		Str("from", row.OptiScalerVersion).Str("to", row.UpgradeTarget).
		Msg("upgrade started")
	if row.Status == domain.StatusExternal {
		// Adopt path: the install backs the external files up SHA-verified
		// first — nothing is uninstalled, and a failed adopt keeps the
		// usual failed-manifest + manual-rollback semantics.
		_ = s.runInstall(gameDir, false, false)
		return
	}
	if row.Status != domain.StatusCommitted {
		return
	}
	// The installer refuses to install over a committed manifest, so the
	// old build is uninstalled first and the new one installed right
	// after. NOT ATOMIC: a crash (or a declined consent gate) between the
	// two steps leaves the game with NO OptiScaler installed; the next
	// scan shows it clean and installable.
	if err := s.runUninstall(gameDir); err != nil {
		return // already surfaced; the old build's state follows uninstall semantics
	}
	if s.upgradeGapHook != nil {
		s.upgradeGapHook(gameDir)
	}
	err := s.runInstall(gameDir, false, false)
	switch {
	case err == nil, errors.Is(err, errInstallPaused), errors.Is(err, context.Canceled):
		// Installed; paused for consent (AnswerConfirm resumes it as a
		// plain install); or cancelled (the installer's cancel path
		// already rolled back atomically).
	default:
		// Install failed AFTER the old build was removed — including
		// errOpBusy (another op grabbed the game's slot in the
		// finishOp→registerOp gap, so this leg never started): run the
		// rollback/backup-restore path so no failed manifest or partial
		// files linger. runRollback re-registers the slot, so a
		// still-busy game refuses gracefully with a busy toast. The
		// install-leg error toast already surfaced.
		log.Warn().Err(err).Str("gameDir", gameDir).
			Msg("upgrade: install failed after uninstall; rolling back")
		s.runRollback(gameDir)
	}
}
