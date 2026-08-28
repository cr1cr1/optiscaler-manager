package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
)

// QuickInstall installs when not installed, uninstalls when installed —
// the one-click toggle from the reference client, with our defaults.
// Version changes are SwitchVersion's job, never the toggle's.
func (s *Session) QuickInstall(gameDir string) {
	row := s.findRow(gameDir)
	if row != nil && row.Status == domain.StatusCommitted {
		go s.doUninstall(gameDir)
		return
	}
	go s.doInstall(gameDir, false, false)
}

// Install starts an install with an explicit EAC override decision.
func (s *Session) Install(gameDir string) {
	go s.doInstall(gameDir, false, false)
}

// Uninstall starts an uninstall.
func (s *Session) Uninstall(gameDir string) {
	go s.doUninstall(gameDir)
}

// Rollback starts a rollback of an interrupted/failed install.
func (s *Session) Rollback(gameDir string) {
	go s.runRollback(gameDir)
}

// runRollback is Rollback's body, callable synchronously by chained ops
// (the version-switch chain runs it as cleanup after a failed install leg).
func (s *Session) runRollback(gameDir string) {
	pre := preOpStatus(s.findRow(gameDir))
	ctx, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return
	}
	s.opStarted("Rolling back…")
	dir, err := app.Rollback(ctx, s.deps.Store, gameDir)
	s.finishOp(gameDir)
	if errors.Is(err, context.Canceled) {
		s.opCancelled(gameDir, pre)
		return
	}
	if errors.Is(err, app.ErrNotManaged) {
		s.opRefused(errNotManagedToast, gameDir)
		return
	}
	if err != nil {
		s.opFailed(err)
		return
	}
	// Rollback restores whatever the adopt-time backup held — possibly a
	// pre-existing external install. Re-probe like doUninstall: external
	// when a branded injection DLL is back, rolled_back otherwise.
	status := domain.StatusRolledBack
	if row := s.findRow(gameDir); row != nil && redetectExternal(row.InjectionDir) == domain.StatusExternal {
		status = domain.StatusExternal
		// The rollback's idempotent job is done: drop the rolled_back
		// manifest so the next scan's enrich probe and the warm-cache
		// reconcile converge on external (exactly like the uninstall
		// path, which deletes its manifest). A later manual Rollback
		// re-run refuses cleanly via ErrNotManaged.
		id, _, err := app.ManifestIDFor(gameDir)
		if err != nil {
			log.Warn().Err(err).Str("dir", gameDir).Msg("rollback: resolve manifest id")
		} else if err := s.deps.Store.Delete(id); err != nil {
			log.Warn().Err(err).Str("id", id).Msg("rollback: drop rolled_back manifest")
		}
	}
	s.setRowStatus(gameDir, status)
	s.opDone("Rolled back "+dir, gameDir)
}

// AnswerConfirm resolves a pending confirmation. Accepted confirmations
// resume the operation with the consented override.
func (s *Session) AnswerConfirm(accept bool) {
	s.mu.Lock()
	c := s.st.Confirm
	s.st.Confirm = nil
	s.mu.Unlock()
	if c == nil || !accept {
		return
	}
	switch c.Kind {
	case ConfirmEAC:
		go s.doInstallVersion(c.GameDir, c.Version, true, false)
	case ConfirmCachedRelease:
		go s.doInstallVersion(c.GameDir, c.Version, false, true)
	case ConfirmVersionSwitch:
		go s.doSwitchVersion(c.GameDir, c.Version, true)
	}
}

func (s *Session) doInstall(gameDir string, eacOK, cachedOK bool) {
	_ = s.runInstall(gameDir, eacOK, cachedOK)
}

// doInstallVersion is doInstall's version-parameterized form: version ""
// keeps the configured default version.
func (s *Session) doInstallVersion(gameDir, version string, eacOK, cachedOK bool) {
	_ = s.runInstallVersion(gameDir, version, eacOK, cachedOK)
}

// runInstall is doInstall's body with an outcome: nil on success,
// errInstallPaused when a consent gate owns the continuation, errOpBusy
// when the game already has an op, the context cause on cancel, and the
// surfaced error on failure. Callers that chain ops (version switch)
// branch on the outcome; fire-and-forget callers discard it.
func (s *Session) runInstall(gameDir string, eacOK, cachedOK bool) error {
	return s.runInstallVersion(gameDir, "", eacOK, cachedOK)
}

// eacWarning is the single source of the anti-cheat consent wording: the
// install gate and the version-switch pre-flight must never drift apart.
func eacWarning(title string) string {
	return fmt.Sprintf("%s uses Easy Anti-Cheat. Installing OptiScaler may result in a ban.", title)
}

// runInstallVersion is runInstall's version-parameterized form: version ""
// installs the configured default (identical behavior in every respect);
// a concrete tag installs exactly that release (the version-switch path).
// The consent gates are version-agnostic: the EAC warning and the
// stale-cache prompt pause with errInstallPaused and resume through
// AnswerConfirm, which carries the tag over via Confirmation.Version, so a
// paused switch never resumes as a different version.
func (s *Session) runInstallVersion(gameDir, version string, eacOK, cachedOK bool) error {
	row := s.findRow(gameDir)
	if row != nil && row.EAC && !eacOK {
		s.setConfirm(&Confirmation{
			Kind:    ConfirmEAC,
			GameDir: gameDir,
			Message: eacWarning(row.Title),
			Version: version,
		})
		return errInstallPaused
	}
	pre := preOpStatus(row)
	ctx, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return errOpBusy
	}
	s.opStarted("Installing…")
	// A scan that just resolved the default live leaves a provably fresh
	// release cache behind; serving it back without the stale-cache
	// consent prompt keeps scan-then-install a one-click flow.
	requested := version
	if requested == "" {
		requested = s.Settings().DefaultVersion
	}
	m, err := app.Install(ctx, s.deps.Store, s.deps.GH, s.deps.CacheDir, gameDir,
		app.InstallOpts{AllowCached: cachedOK || s.defaultRecentlyResolved(), EACOverride: eacOK, Requested: requested})
	s.finishOp(gameDir)
	if errors.Is(err, context.Canceled) {
		s.opCancelled(gameDir, pre)
		return err
	}
	if errors.Is(err, app.ErrStaleCache) {
		s.opAborted()
		s.setConfirm(&Confirmation{
			Kind:    ConfirmCachedRelease,
			GameDir: gameDir,
			Message: "GitHub is rate-limiting; only stale cached release info is available. Use it anyway?",
			Version: version,
		})
		return errInstallPaused
	}
	if err != nil {
		s.opFailed(err)
		return err
	}
	s.setRowInstalled(gameDir, m.Resolved.Version)
	s.opDone("Installed "+gameTitle(row, gameDir), gameDir)
	return nil
}

// errNotManagedToast is the user-facing refusal when an uninstall targets an
// install this manager never made (external, or its manifest vanished): the
// store holds no manifest, so no SHA-verified removal is possible and the
// raw store sentinel must never leak into a toast.
const errNotManagedToast = "not installed by this manager — adopt first or remove manually"

func (s *Session) doUninstall(gameDir string) {
	_ = s.runUninstall(gameDir)
}

// runUninstall is doUninstall's body with an outcome, mirroring
// runInstall: nil on success, app.ErrNotManaged on the refusal paths,
// errOpBusy when the game is busy, the context cause on cancel, and the
// surfaced error on failure.
func (s *Session) runUninstall(gameDir string) error {
	row := s.findRow(gameDir)
	if row != nil && row.Status == domain.StatusExternal {
		s.toast(errNotManagedToast, true)
		return app.ErrNotManaged
	}
	pre := preOpStatus(row)
	ctx, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return errOpBusy
	}
	s.opStarted("Uninstalling…")
	_, err := app.Uninstall(ctx, s.deps.Store, gameDir)
	s.finishOp(gameDir)
	if errors.Is(err, context.Canceled) {
		s.opCancelled(gameDir, pre)
		return err
	}
	if errors.Is(err, app.ErrNotManaged) {
		s.opRefused(errNotManagedToast, gameDir)
		return err
	}
	if err != nil {
		s.opFailed(err)
		return err
	}
	// Uninstall restores whatever the adopt-time backup held — possibly a
	// pre-existing external install. One bounded probe keeps the row honest:
	// external when a branded injection DLL is back, "" when the dir is clean.
	var injectionDir string
	if row != nil {
		injectionDir = row.InjectionDir
	}
	s.setRowStatus(gameDir, redetectExternal(injectionDir))
	s.opDone("Uninstalled "+gameTitle(s.findRow(gameDir), gameDir), gameDir)
	return nil
}

// redetectExternal runs the bounded post-op probe shared by uninstall and
// rollback: external when a branded injection DLL is (back) in injectionDir,
// "" otherwise. An empty injectionDir probes nothing.
func redetectExternal(injectionDir string) domain.Status {
	if injectionDir == "" {
		return ""
	}
	if found, _ := pever.DetectOptiScaler(injectionDir); found {
		return domain.StatusExternal
	}
	return ""
}

func preOpStatus(row *GameRow) domain.Status {
	if row != nil {
		return row.Status
	}
	return ""
}

// registerOp records a cancellable context for gameDir, serializing ops per
// game: false (and no context) when one is already in flight.
func (s *Session) registerOp(gameDir string) (context.Context, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.opCancels[gameDir]; busy {
		return nil, false
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.opCancels[gameDir] = cancel
	return ctx, true
}

// finishOp releases the op slot for gameDir. It runs before any terminal
// event is emitted so a follow-up op never sees a stale slot.
func (s *Session) finishOp(gameDir string) {
	s.mu.Lock()
	if cancel, ok := s.opCancels[gameDir]; ok {
		delete(s.opCancels, gameDir)
		cancel()
	}
	s.mu.Unlock()
}

// CancelOp cancels the in-flight install/uninstall/rollback for gameDir and
// reports whether one was running.
func (s *Session) CancelOp(gameDir string) bool {
	s.mu.Lock()
	cancel, ok := s.opCancels[gameDir]
	s.mu.Unlock()
	if ok {
		log.Info().Str("gameDir", gameDir).Msg("cancelling op")
		cancel()
	}
	return ok
}

// OpBusy reports whether gameDir has an op in flight. Frontends gate
// per-game cancel affordances on it (a global busy flag would point the
// button at the wrong game).
func (s *Session) OpBusy(gameDir string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, busy := s.opCancels[gameDir]
	return busy
}

func gameTitle(row *GameRow, dir string) string {
	if row != nil {
		return row.Title
	}
	return dir
}
