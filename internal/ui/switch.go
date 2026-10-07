package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// forkSlugOrDefault normalizes a manifest/row fork slug: legacy installs
// predate fork tracking and read as upstream.
func forkSlugOrDefault(slug string) string {
	if slug == "" {
		return settings.DefaultForkSlug
	}
	return slug
}

// forkBackupDir names the fork-switch preservation dir: <repo>.YYMMDD
// (repo segment of the old fork's slug, local date) inside the injection
// dir. A same-day repeat gets -2, -3, … so an earlier backup is never
// clobbered.
func forkBackupDir(installDir, oldFork string, now time.Time) string {
	name := forkSlugOrDefault(oldFork)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	base := filepath.Join(installDir, name+"."+now.Format("060102"))
	dir := base
	for n := 2; n < 100; n++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return dir
		}
		dir = fmt.Sprintf("%s-%d", base, n)
	}
	return dir
}

// SwitchVersion starts a per-game switch to a chosen OptiScaler version:
// the version-parameterized sibling of Upgrade. version may be the literal
// "latest" (the dropdown's Latest option), which re-resolves to the newest
// release at pick time before the chain starts. The game's existing
// OptiScaler.ini is captured first and written back after the install leg
// succeeds — the installer would otherwise replace it with the curated
// defaults (applyCuratedINI), and keeping the user's tuning is the whole
// point of switching versions in place.
func (s *Session) SwitchVersion(gameDir, version string) {
	go s.doSwitchVersion(gameDir, version, false)
}

// doSwitchVersion wraps the switch chain with the compound op's single
// terminal event: whatever the chain's outcome, EvOpSettled fires exactly
// once at the very end with the outcome text (an empty chain return — a
// paused consent gate — settles nothing; the gate's continuation runs the
// wrapper again and settles there).
func (s *Session) doSwitchVersion(gameDir, version string, eacConsented bool) {
	if settled := s.doSwitchVersionChain(gameDir, version, eacConsented); settled != "" {
		s.emit(Event{Kind: EvOpSettled, Text: settled, GameDir: gameDir})
	}
}

// doSwitchVersionChain is the switch chain proper; it returns the settle
// text for the wrapper's EvOpSettled ("" when the chain paused at a
// consent gate or was mis-dispatched).
//
// The chain mirrors doUpgrade's proven shape — committed rows chain
// uninstall-then-install (rollback on failure-after-uninstall), external
// rows adopt-install directly — with two differences: the install leg
// targets the CHOSEN tag instead of the resolved default, and the captured
// ini is restored on success. Unknown games, rows without a switchable
// install, and a switch to the ALREADY installed version are silent no-ops
// for the frontends (the version picker raced a state change or
// re-selected the current entry): nothing is dispatched, no mid-chain
// events fire — the settle text still reports the no-op so the CLI's
// one-shot waiter ends deterministically.
//
// eacConsented is true only when the chain resumes from an answered
// ConfirmVersionSwitch prompt: the EAC gate is pre-flighted BEFORE any
// destructive step (ini capture/removal, uninstall), because a consent
// pause mid-chain used to strand the game uninstalled with the ini already
// deleted — pure data loss on decline, and a curated-defaults install on
// accept. With the pre-flight, decline means zero operation ran.
func (s *Session) doSwitchVersionChain(gameDir, version string, eacConsented bool) string {
	if version == "" {
		return ""
	}
	if version == "latest" {
		// The dropdown's "Latest" option dispatches the literal: re-resolve
		// the newest release at pick time (the startup memo may be stale)
		// and proceed with the CONCRETE tag — the same-version no-op guard,
		// the EAC consent pin, and the install leg all see a real tag.
		resolve := s.resolveVersion
		if resolve == nil {
			resolve = s.ghResolveVersion
		}
		tag, _, err := resolve(context.Background(), "latest")
		if err != nil || tag == "" {
			log.Warn().Err(err).Msg("version switch: cannot resolve the latest release")
			s.toast("cannot resolve the latest OptiScaler version", true)
			return "cannot resolve the latest OptiScaler version"
		}
		version = tag
	}
	row := s.findRow(gameDir)
	if row == nil {
		return "unknown game dir"
	}
	if row.OptiScalerVersion == version {
		return "already at " + version
	}
	if row.Status != domain.StatusExternal && row.Status != domain.StatusCommitted {
		return "game is not installed; nothing to switch"
	}
	log.Info().Str("gameDir", gameDir).
		Str("from", row.OptiScalerVersion).Str("to", version).
		Msg("version switch started")
	if row.EAC && !eacConsented {
		s.setConfirm(&Confirmation{
			Kind:    ConfirmVersionSwitch,
			GameDir: gameDir,
			Message: eacWarning(row.Title),
			Version: version,
		})
		return "" // paused at the gate; the answered resume settles
	}
	ini, err := s.captureINIDurable(gameDir, row.InjectionDir)
	if err != nil {
		// The crash-durable copy could not be written: abort BEFORE the
		// ini is removed — continuing would re-open the crash window where
		// the bytes live only in memory.
		log.Warn().Err(err).Str("gameDir", gameDir).
			Msg("version switch: OptiScaler.ini crash backup failed; aborting before any destructive step")
		s.toast("Version switch aborted: could not back up your OptiScaler.ini", true)
		return "switch failed: could not back up your OptiScaler.ini"
	}
	if row.Status == domain.StatusExternal {
		// Adopt path (mirrors doUpgrade): the install backs the external
		// files up SHA-verified first — nothing is uninstalled, and a
		// failed adopt keeps the usual failed-manifest + manual-rollback
		// semantics.
		if err := s.runInstallVersion(gameDir, version, installConsent{eacOK: eacConsented, cachedOK: true}); err != nil {
			return "switch failed: " + err.Error()
		}
		if s.restoreINI(gameDir, ini) == nil {
			ini.discardBackup()
		}
		return "switched to " + version
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
	//
	// Fork switch (issue 10): when the installed distribution differs
	// from the active source, the old fork's whole file set moves into
	// <repo>.YYMMDD inside the injection dir instead of being deleted —
	// the archive-derived manifest file set is exactly what gets moved.
	relocateDir := ""
	if forkSlugOrDefault(row.Fork) != s.Settings().Active().Slug {
		relocateDir = forkBackupDir(row.InjectionDir, row.Fork, s.now())
	}
	if err := s.runUninstallOpt(gameDir, relocateDir); err != nil {
		if removedINI {
			// Nothing else ran (refused or busy): put the ini back so
			// the game is exactly as found.
			if werr := writeINI(ini.path, ini); werr != nil {
				log.Warn().Err(werr).Str("gameDir", gameDir).
					Msg("version switch: OptiScaler.ini write-back after refused uninstall failed")
			} else {
				ini.discardBackup()
			}
		}
		return "switch failed: " + err.Error() // the uninstall error was already surfaced
	}
	if relocateDir != "" {
		s.toast("Old fork files saved to "+filepath.Base(relocateDir), false)
	}
	if s.upgradeGapHook != nil {
		s.upgradeGapHook(gameDir)
	}
	// cachedOK: the user pinned a concrete tag, so stale release metadata
	// cannot resolve to the WRONG version the way a stale "latest" can —
	// and the chosen bundle may legitimately come from the local cache.
	err = s.runInstallVersion(gameDir, version, installConsent{eacOK: eacConsented, cachedOK: true})
	switch {
	case err == nil:
		if s.restoreINI(gameDir, ini) == nil {
			ini.discardBackup()
		}
		return "switched to " + version
	case errors.Is(err, errInstallPaused):
		// Unreachable with the EAC pre-flight and cachedOK=true (the
		// staged install gate's AnswerConfirm continuation runs
		// doInstallVersion directly, outside this chain — no ini
		// write-back, no settle from there). Settle with a failure text
		// so a CLI waiter can never wait out its timeout on this branch.
		return "switch failed: consent gate resumed outside the switch chain"
	case errors.Is(err, context.Canceled):
		// The installer's cancel path rolled its partial install back
		// atomically, but the OLD build was already uninstalled: the game
		// is clean, so the captured ini goes back as an orphan.
		s.writeBackOrphanINI(gameDir, ini)
		return "switch cancelled"
	default:
		// Same contract as doUpgrade: install failed AFTER the old build
		// was removed — including errOpBusy — so run the
		// rollback/backup-restore path; no failed manifest or partial
		// files may linger. The install-leg error toast already surfaced.
		log.Warn().Err(err).Str("gameDir", gameDir).
			Msg("version switch: install failed after uninstall; rolling back")
		s.runRollback(gameDir)
		// The rollback leaves the game CLEAN: write the captured ini back
		// as an orphan — it is the only copy of the user's tuning.
		s.writeBackOrphanINI(gameDir, ini)
		return "switch failed: install failed; rolled back"
	}
}

// preservedINI holds a game's OptiScaler.ini across a version switch:
// bytes plus the original mode, so the write-back is byte- AND
// permission-identical. backup is the crash-durable copy under the
// session data root ("" when no ini existed to preserve).
type preservedINI struct {
	path   string
	data   []byte
	mode   os.FileMode
	backup string
	ok     bool
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

// captureINIDurable is captureINI plus a crash-durable copy: the switch
// removes the ini from disk before the uninstall leg, and until the
// write-back the bytes would otherwise live only in process memory — an
// app crash in that window destroys the user's tuning. The copy lands
// under the session data root, named per game dir, and discardBackup
// removes it once the ini is safely back in place. A failed copy aborts
// the switch before anything destructive.
func (s *Session) captureINIDurable(gameDir, injectionDir string) (preservedINI, error) {
	ini := captureINI(injectionDir)
	if !ini.ok {
		return ini, nil
	}
	root := s.deps.SettingsRoot
	if root == "" {
		// Production always wires a data root (cmd/session.go); tests may
		// not, and the crash window must still be covered.
		root = os.TempDir()
	}
	backup := iniSwitchBackupPath(root, gameDir)
	if err := os.WriteFile(backup, ini.data, ini.mode); err != nil {
		return ini, err
	}
	ini.backup = backup
	return ini, nil
}

// iniSwitchBackupPath names the per-game crash copy: the sha256 of the
// game dir keeps it unique and collision-free inside the data root.
func iniSwitchBackupPath(root, gameDir string) string {
	sum := sha256.Sum256([]byte(gameDir))
	return filepath.Join(root, "ini-switch-backup-"+hex.EncodeToString(sum[:])+".tmp")
}

// discardBackup removes the crash-durable copy after the ini is safely
// back in place. A lingering copy is harmless (the next switch for the
// same game overwrites it), so cleanup failure only logs.
func (ini preservedINI) discardBackup() {
	if ini.backup == "" {
		return
	}
	if err := os.Remove(ini.backup); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Str("backup", ini.backup).
			Msg("version switch: OptiScaler.ini crash-backup cleanup failed")
	}
}

// writeBackOrphanINI returns the captured ini to a game dir the chain left
// CLEAN (rollback after a failed install leg, or a cancelled install after
// the uninstall already ran): an orphan OptiScaler.ini preserves the
// user's tuning for the next install. The crash backup is kept when the
// write-back itself fails — it is the last copy.
func (s *Session) writeBackOrphanINI(gameDir string, ini preservedINI) {
	if !ini.ok {
		return
	}
	if err := writeINI(ini.path, ini); err != nil {
		log.Warn().Err(err).Str("gameDir", gameDir).Str("ini", ini.path).
			Msg("version switch: OptiScaler.ini write-back after cleanup failed; crash backup retained")
		return
	}
	ini.discardBackup()
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
// curated ini. The error return lets callers keep the crash-durable backup
// when the ini did NOT make it back.
func (s *Session) restoreINI(gameDir string, ini preservedINI) error {
	if !ini.ok {
		return nil
	}
	if s.switchINIHook != nil {
		s.switchINIHook(gameDir)
	}
	row := s.findRow(gameDir)
	if row == nil || row.InjectionDir == "" {
		err := errors.New("game row vanished mid-switch")
		log.Warn().Err(err).Str("gameDir", gameDir).
			Msg("version switch: OptiScaler.ini restore impossible; crash backup retained")
		return err
	}
	path := filepath.Join(row.InjectionDir, "OptiScaler.ini")
	if err := writeINI(path, ini); err != nil {
		log.Warn().Err(err).Str("gameDir", gameDir).Str("ini", path).
			Msg("version switch: OptiScaler.ini restore failed; curated defaults kept")
		s.toast("Installed, but restoring your OptiScaler.ini failed — curated defaults in place", true)
		return err
	}
	return nil
}
