package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// SwitchVersion starts a per-game switch to a chosen OptiScaler version:
// the version-parameterized sibling of Upgrade. The game's existing
// OptiScaler.ini is captured first and written back after the install leg
// succeeds — the installer would otherwise replace it with the curated
// defaults (applyCuratedINI), and keeping the user's tuning is the whole
// point of switching versions in place.
func (s *Session) SwitchVersion(gameDir, version string) {
	go s.doSwitchVersion(gameDir, version)
}

// doSwitchVersion mirrors doUpgrade's proven chain — committed rows chain
// uninstall-then-install (rollback on failure-after-uninstall), external
// rows adopt-install directly — with two differences: the install leg
// targets the CHOSEN tag instead of the resolved default, and the captured
// ini is restored on success. Unknown games, rows without a switchable
// install, and a switch to the ALREADY installed version are silent no-ops
// (the version picker raced a state change or re-selected the current
// entry): nothing is dispatched, no events fire.
func (s *Session) doSwitchVersion(gameDir, version string) {
	if version == "" {
		return
	}
	row := s.findRow(gameDir)
	if row == nil || row.OptiScalerVersion == version {
		return
	}
	log.Info().Str("gameDir", gameDir).
		Str("from", row.OptiScalerVersion).Str("to", version).
		Msg("version switch started")
	ini := captureINI(row.InjectionDir)
	if row.Status == domain.StatusExternal {
		// Adopt path (mirrors doUpgrade): the install backs the external
		// files up SHA-verified first — nothing is uninstalled, and a
		// failed adopt keeps the usual failed-manifest + manual-rollback
		// semantics.
		if err := s.runInstallVersion(gameDir, version, false, true); err == nil {
			s.restoreINI(gameDir, ini)
		}
		return
	}
	if row.Status != domain.StatusCommitted {
		return
	}
	// A user-edited OptiScaler.ini makes the uninstaller REFUSE (foreign
	// modifications are never deleted) — and a customized ini is exactly
	// the file a switch must preserve. Holding the bytes in memory and
	// removing the file up front lets the uninstaller treat it as
	// already-vanished (skipped, not refused); the write-back after the
	// install leg puts the user's tuning back.
	removedINI := false
	if ini.ok {
		removedINI = os.Remove(ini.path) == nil
	}
	// NOT ATOMIC (same as doUpgrade): the installer refuses to install
	// over a committed manifest, so the old build is uninstalled first;
	// a crash between the legs leaves the game clean and installable.
	if err := s.runUninstall(gameDir); err != nil {
		if removedINI {
			// Nothing else ran (refused or busy): put the ini back so
			// the game is exactly as found.
			if werr := writeINI(ini.path, ini); werr != nil {
				log.Warn().Err(werr).Str("gameDir", gameDir).
					Msg("version switch: OptiScaler.ini write-back after refused uninstall failed")
			}
		}
		return // the uninstall error was already surfaced (incl. errOpBusy)
	}
	if s.upgradeGapHook != nil {
		s.upgradeGapHook(gameDir)
	}
	// cachedOK: the user pinned a concrete tag, so stale release metadata
	// cannot resolve to the WRONG version the way a stale "latest" can —
	// and the chosen bundle may legitimately come from the local cache.
	err := s.runInstallVersion(gameDir, version, false, true)
	switch {
	case err == nil:
		s.restoreINI(gameDir, ini)
	case errors.Is(err, errInstallPaused), errors.Is(err, context.Canceled):
		// Paused for consent (AnswerConfirm resumes a plain install at
		// the same version via Confirmation.Version) or cancelled (the
		// installer's cancel path already rolled back atomically).
	default:
		// Same contract as doUpgrade: install failed AFTER the old build
		// was removed — including errOpBusy — so run the
		// rollback/backup-restore path; no failed manifest or partial
		// files may linger. The install-leg error toast already surfaced.
		log.Warn().Err(err).Str("gameDir", gameDir).
			Msg("version switch: install failed after uninstall; rolling back")
		s.runRollback(gameDir)
	}
}

// preservedINI holds a game's OptiScaler.ini across a version switch:
// bytes plus the original mode, so the write-back is byte- AND
// permission-identical.
type preservedINI struct {
	path string
	data []byte
	mode os.FileMode
	ok   bool
}

// captureINI reads the OptiScaler.ini in the injection dir, if any. A
// missing or unreadable ini is not an error: there is simply nothing to
// preserve, and the curated defaults the install leg drops are correct.
func captureINI(injectionDir string) preservedINI {
	if injectionDir == "" {
		return preservedINI{}
	}
	path := filepath.Join(injectionDir, "OptiScaler.ini")
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return preservedINI{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return preservedINI{}
	}
	return preservedINI{path: path, data: data, mode: st.Mode().Perm(), ok: true}
}

// writeINI writes captured ini bytes back with the original mode.
// WriteFile keeps an existing file's mode, so the captured mode is
// re-applied explicitly — the write-back must be permission-identical,
// not just byte-identical.
func writeINI(path string, ini preservedINI) error {
	err := os.WriteFile(path, ini.data, ini.mode)
	if err == nil {
		err = os.Chmod(path, ini.mode)
	}
	return err
}

// restoreINI writes the captured ini back over the curated defaults the
// install leg just dropped. The install already stands committed, so a
// failed write-back must NOT roll it back or crash the op: the failure
// surfaces as a warning toast plus a zerolog warn, and the game keeps the
// curated ini.
func (s *Session) restoreINI(gameDir string, ini preservedINI) {
	if !ini.ok {
		return
	}
	if s.switchINIHook != nil {
		s.switchINIHook(gameDir)
	}
	row := s.findRow(gameDir)
	if row == nil || row.InjectionDir == "" {
		return
	}
	path := filepath.Join(row.InjectionDir, "OptiScaler.ini")
	if err := writeINI(path, ini); err != nil {
		log.Warn().Err(err).Str("gameDir", gameDir).Str("ini", path).
			Msg("version switch: OptiScaler.ini restore failed; curated defaults kept")
		s.toast("Installed, but restoring your OptiScaler.ini failed — curated defaults in place", true)
	}
}
