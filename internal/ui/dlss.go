package ui

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
)

// UpdateDLSS starts an explicit update of the existing three-file NVIDIA
// runtime set (DLSS, DLSSD, DLSS-G) from NVIDIA's official repository. It
// never adds a missing DLL: every member must already exist in the game's
// injection directory.
func (s *Session) UpdateDLSS(gameDir string) { go s.doUpdateDLSS(gameDir) }

func (s *Session) doUpdateDLSS(gameDir string) {
	pre := preOpStatus(s.findRow(gameDir))
	ctx, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return
	}
	s.opStarted("Updating NVIDIA DLSS…")
	_, err := app.UpdateDLSS(ctx, s.deps.DLSS, s.deps.SettingsRoot, gameDir)
	s.finishOp(gameDir)
	switch {
	case errors.Is(err, context.Canceled):
		s.opCancelled(gameDir, pre)
	case err != nil:
		s.opFailed(err)
	default:
		s.refreshComponentVersions(gameDir)
		s.opDone("Updated NVIDIA DLSS", gameDir)
	}
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
	pre := preOpStatus(s.findRow(gameDir))
	ctx, ok := s.registerOp(gameDir)
	if !ok {
		s.toast("operation already in progress for this game", true)
		return
	}
	s.opStarted("Restoring NVIDIA DLSS…")
	_, err := app.RestoreDLSS(ctx, s.deps.SettingsRoot, gameDir, snapshotID)
	s.finishOp(gameDir)
	switch {
	case errors.Is(err, context.Canceled):
		s.opCancelled(gameDir, pre)
	case err != nil:
		s.opFailed(err)
	default:
		s.refreshComponentVersions(gameDir)
		s.opDone("Restored NVIDIA DLSS", gameDir)
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

// componentLabels orders the version map exactly like scan's toRow does
// (alphabetical keys: dlss, fsr, xess).
func componentLabels(versions map[string]string) []string {
	keys := make([]string, 0, len(versions))
	for k := range versions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, versions[k])
	}
	return out
}
