package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
)

// UpdateDLSS starts an explicit update of the three-file NVIDIA runtime set
// (DLSS, DLSSD, DLSS-G) from NVIDIA's official repository. A complete
// current set is backed up first; a partial or absent current set only logs
// a warning and the update proceeds without a rollback backup, installing
// the missing members with the rest. The update is cache-first: when the
// startup check's published commit is already in the download cache the
// install runs with zero network; otherwise the missing members are fetched
// into that cache and the install always serves from the cache dir.
func (s *Session) UpdateDLSS(gameDir string) { go s.doUpdateDLSS(gameDir) }

func (s *Session) doUpdateDLSS(gameDir string) {
	hint := func() string {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.st.DLSSLatest.Commit != "" {
			return s.st.DLSSLatest.Commit
		}
		return s.st.DLSSCachedCommit
	}()
	s.runDLSSOp(gameDir, "Updating NVIDIA DLSS…", func(ctx context.Context) error {
		_, err := app.UpdateDLSS(ctx, s.deps.DLSS, s.deps.CacheDir, gameDir, hint)
		return err
	}, "Updated NVIDIA DLSS")
}

// CheckDLSS is the startup availability check, called once at boot (and by
// tests directly): the cached half is a local PE read of the download
// cache, the online half one small tags-API call that resolves the
// published version and the commit it points at for the update hint — and
// pre-downloads that latest published set into the cache when it is not
// already cached (user spec: startup fetches `latest`; zero network when
// cached). Failures degrade silently: the state stays zero and the update
// flow still works by resolving online at press time.
func (s *Session) CheckDLSS(ctx context.Context) {
	s.mu.Lock()
	cached, commit := dlss.CachedVersion(s.deps.CacheDir)
	s.setDLSSCachedLocked(cached, commit)
	s.mu.Unlock()
	if s.deps.DLSS == nil {
		return
	}
	go func() {
		if !s.Settings().OnlineLookups {
			return
		}
		lat, err := s.deps.DLSS.Latest(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("dlss: startup published-version check failed")
			return
		}
		s.mu.Lock()
		s.st.DLSSLatest = lat
		s.mu.Unlock()
		s.emit(Event{Kind: EvDLSSStatus, Text: lat.Version})
		// User spec: startup also pre-downloads the latest published set
		// into the cache (zero network when the commit is already cached)
		// so the DLSS pill serves the latest offline-ready.
		if lat.Commit != "" {
			if err := s.deps.DLSS.Preload(ctx, s.deps.CacheDir, lat.Commit); err != nil {
				log.Warn().Err(err).Msg("dlss: startup latest preload failed")
			}
		}
	}()
}

// setDLSSCached records the newest version the local download cache holds
// (and the commit dir that serves it) under the session lock. CheckDLSS
// holds the lock for the cached half itself; refreshComponentVersions
// calls the locked variant.
func (s *Session) setDLSSCached(v, commit string) {
	s.mu.Lock()
	s.setDLSSCachedLocked(v, commit)
	s.mu.Unlock()
}

func (s *Session) setDLSSCachedLocked(v, commit string) {
	s.st.DLSSCached = v
	s.st.DLSSCachedCommit = commit
}

// DLSSSnapshots returns the complete backed-up NVIDIA runtime sets that can
// be restored, newest first.
func (s *Session) DLSSSnapshots(gameDir string) []dlss.Snapshot {
	snaps, err := app.DLSSSnapshots(gameDir)
	if err != nil {
		return nil
	}
	return snaps
}

// RestoreDLSS asks for confirmation before restoring the selected complete
// NVIDIA runtime set; declining leaves everything untouched.
func (s *Session) RestoreDLSS(gameDir, snapshotID string) {
	for _, snap := range s.DLSSSnapshots(gameDir) {
		if snap.ID == snapshotID {
			s.setConfirm(&Confirmation{
				Kind:       ConfirmDLSSRestore,
				GameDir:    gameDir,
				SnapshotID: snapshotID,
				Message: fmt.Sprintf("Restore the NVIDIA DLSS set backed up %s? A complete current set is backed up first.",
					snap.CreatedAt.Local().Format("2006-01-02 15:04")),
			})
			return
		}
	}
	s.toast("DLSS backup no longer exists", true)
}

func (s *Session) doRestoreDLSS(gameDir, snapshotID string) {
	s.runDLSSOp(gameDir, "Restoring NVIDIA DLSS…", func(ctx context.Context) error {
		_, err := app.RestoreDLSS(ctx, gameDir, snapshotID)
		return err
	}, "Restored NVIDIA DLSS")
}

// runDLSSOp is the shared shell of the NVIDIA DLSS operations: register the
// per-game op slot, run under it, settle. Success always re-probes the
// row's component versions so the DLSS label updates immediately.
func (s *Session) runDLSSOp(gameDir, started string, run func(ctx context.Context) error, done string) {
	pre := preOpStatus(s.findRow(gameDir))
	ctx, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return
	}
	s.opStarted(started)
	err := run(ctx)
	s.finishOp(gameDir)
	var already *dlss.AlreadyLatestError
	switch {
	case errors.Is(err, context.Canceled):
		s.opCancelled(gameDir, pre)
	case errors.As(err, &already):
		// Nothing was installed, nothing changed: settle as an
		// informational success naming the version, not a failure.
		s.opDone(fmt.Sprintf("NVIDIA DLSS already at %s", already.Version), gameDir)
	case err != nil:
		s.opFailed(err, gameDir)
	default:
		s.refreshComponentVersions(gameDir)
		s.opDone(done, gameDir)
	}
}

// refreshComponentVersions re-probes the row's upscaler DLL versions from
// disk so the DLSS pill shows the just-updated (or restored) version, and
// refreshes the row's DLSS readiness + raw version with it. The session's
// cached-version status is refreshed too: the update just re-filled the
// download cache.
func (s *Session) refreshComponentVersions(gameDir string) {
	row := s.findRow(gameDir)
	if row == nil || row.InjectionDir == "" {
		return
	}
	components, raw := app.ComponentVersions(row.InjectionDir)
	labels := componentLabels(components)
	ready := dlss.Complete(row.InjectionDir)
	s.mu.Lock()
	for i := range s.st.Rows {
		if s.st.Rows[i].InstallDir == gameDir {
			s.st.Rows[i].Components = labels
			s.st.Rows[i].DLSSVersion = raw
			s.st.Rows[i].DLSSReady = ready
			break
		}
	}
	s.mu.Unlock()
	cached, commit := dlss.CachedVersion(s.deps.CacheDir)
	s.setDLSSCached(cached, commit)
	s.persistCache()
}
