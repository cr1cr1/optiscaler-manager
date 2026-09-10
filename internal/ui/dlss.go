package ui

import (
	"context"
	"errors"
	"fmt"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
)

// UpdateDLSS starts an explicit update of the existing three-file NVIDIA
// runtime set (DLSS, DLSSD, DLSS-G) from NVIDIA's official repository. It
// never adds a missing DLL: every member must already exist in the game's
// injection directory.
func (s *Session) UpdateDLSS(gameDir string) { go s.doUpdateDLSS(gameDir) }

func (s *Session) doUpdateDLSS(gameDir string) {
	s.runDLSSOp(gameDir, "Updating NVIDIA DLSS…", func(ctx context.Context) error {
		_, err := app.UpdateDLSS(ctx, s.deps.DLSS, s.deps.CacheDir, s.deps.SettingsRoot, gameDir)
		return err
	}, "Updated NVIDIA DLSS")
}

// DLSSSnapshots returns the complete backed-up NVIDIA runtime sets that can
// be restored, newest first.
func (s *Session) DLSSSnapshots(gameDir string) []dlss.Snapshot {
	snaps, err := app.DLSSSnapshots(s.deps.SettingsRoot, gameDir)
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
				Message: fmt.Sprintf("Restore the NVIDIA DLSS set backed up %s? The current DLLs are backed up first.",
					snap.CreatedAt.Local().Format("2006-01-02 15:04")),
			})
			return
		}
	}
	s.toast("DLSS backup no longer exists", true)
}

func (s *Session) doRestoreDLSS(gameDir, snapshotID string) {
	s.runDLSSOp(gameDir, "Restoring NVIDIA DLSS…", func(ctx context.Context) error {
		_, err := app.RestoreDLSS(ctx, s.deps.SettingsRoot, gameDir, snapshotID)
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
	switch {
	case errors.Is(err, context.Canceled):
		s.opCancelled(gameDir, pre)
	case err != nil:
		s.opFailed(err)
	default:
		s.refreshComponentVersions(gameDir)
		s.opDone(done, gameDir)
	}
}

// refreshComponentVersions re-probes the row's upscaler DLL versions from
// disk so the DLSS pill shows the just-updated (or restored) version.
func (s *Session) refreshComponentVersions(gameDir string) {
	row := s.findRow(gameDir)
	if row == nil || row.InjectionDir == "" {
		return
	}
	components := app.ComponentVersions(row.InjectionDir)
	s.mu.Lock()
	for i := range s.st.Rows {
		if s.st.Rows[i].InstallDir == gameDir {
			s.st.Rows[i].Components = componentLabels(components)
			break
		}
	}
	s.mu.Unlock()
	s.persistCache()
}
