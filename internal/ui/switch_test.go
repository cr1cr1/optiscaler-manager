package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// customINIBytes is a marker OptiScaler.ini the curated defaults never
// contain: byte-equality after a switch proves the user's tuning survived.
var customINIBytes = []byte("[Upscalers]\ncustom=1\n")

// TestSwitchCommittedPreservesCustomINI (S1): switching a committed game to
// another version chains uninstall-then-install at the CHOSEN tag, and the
// game's own OptiScaler.ini (bytes AND mode) survives — the install leg
// drops curated defaults over it (installer applyCuratedINI), so the
// session must write the captured bytes back.
func TestSwitchCommittedPreservesCustomINI(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)

	iniPath := filepath.Join(e.bin, "OptiScaler.ini")
	if err := os.WriteFile(iniPath, customINIBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	e.sess.SwitchVersion(e.gameRoot, "v0.10.0-test")
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Uninstalled") {
		t.Fatalf("first settle = %q, want the uninstall leg first", ev.Text)
	}
	ev = waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Installed") {
		t.Fatalf("second settle = %q, want the install leg second", ev.Text)
	}

	row := theRow(t, e.sess)
	if row.Status != domain.StatusCommitted {
		t.Fatalf("row status after switch = %q, want committed", row.Status)
	}
	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	if manifests[0].Resolved.Version != "v0.10.0-test" {
		t.Errorf("manifest version = %q, want v0.10.0-test", manifests[0].Resolved.Version)
	}
	got, err := os.ReadFile(iniPath)
	if err != nil {
		t.Fatalf("OptiScaler.ini missing after switch: %v", err)
	}
	if !bytes.Equal(got, customINIBytes) {
		t.Errorf("ini bytes after switch = %q, want the custom bytes (curated defaults won)", got)
	}
	if st, err := os.Stat(iniPath); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("ini mode after switch = %v (err %v), want 0600", st.Mode().Perm(), err)
	}
	t.Log("S1: committed switch landed at the chosen version with the custom ini intact")
}

// TestSwitchExternalAdoptsAtChosenVersion (S2): an external install is never
// uninstalled; the switch adopt-installs at the CHOSEN tag (here the OLDER
// one, proving version-parameterization against a "latest" default), backs
// the external files up SHA-verified, and preserves the external ini.
func TestSwitchExternalAdoptsAtChosenVersion(t *testing.T) {
	e := newUpgradeEnv(t, "latest")
	marker := writeExternalMarker(t, e.bin)
	writeUIFile(t, filepath.Join(e.bin, "OptiScaler.ini"), string(customINIBytes))

	scanAndWait(t, e.sess)
	row := theRow(t, e.sess)
	if row.Status != domain.StatusExternal {
		t.Fatalf("row status = %q, want external", row.Status)
	}

	e.sess.SwitchVersion(e.gameRoot, "v0.9.4-test")
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Installed") {
		t.Fatalf("settle event = %q, want a direct adopt install (no uninstall leg)", ev.Text)
	}
	for _, toast := range e.sess.Snapshot().Toasts {
		if strings.Contains(toast.Text, "Uninstalled") || strings.Contains(toast.Text, notManagedRefusal) {
			t.Fatalf("external switch touched the uninstall path: %q", toast.Text)
		}
	}

	row = theRow(t, e.sess)
	if row.Status != domain.StatusCommitted {
		t.Fatalf("row status after adopt-switch = %q, want committed", row.Status)
	}
	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	if manifests[0].Resolved.Version != "v0.9.4-test" {
		t.Errorf("manifest version = %q, want v0.9.4-test (the CHOSEN tag, not the default latest)",
			manifests[0].Resolved.Version)
	}
	// The adopt path's backup holds the exact external bytes.
	backup := filepath.Join(e.store.BackupDir(manifests[0].ID), "files", "dxgi.dll")
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("external backup missing: %v", err)
	}
	if string(data) != string(marker) {
		t.Error("external backup bytes differ from the planted marker")
	}
	got, err := os.ReadFile(filepath.Join(e.bin, "OptiScaler.ini"))
	if err != nil {
		t.Fatalf("OptiScaler.ini missing after adopt-switch: %v", err)
	}
	if !bytes.Equal(got, customINIBytes) {
		t.Errorf("ini bytes after adopt-switch = %q, want the custom bytes", got)
	}
	t.Log("S2: external adopt-switch at the chosen older tag, marker SHA-backed-up, ini preserved")
}

// TestSwitchInstallLegFailureRollsBack (S7): when the install leg fails
// after the old build was already uninstalled, the rollback/backup-restore
// path runs exactly as doUpgrade's does — an error toast surfaces, no
// failed manifest lingers, no partial bundle files survive.
func TestSwitchInstallLegFailureRollsBack(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)

	// A customized ini must survive the rollback too: the failed install
	// leg leaves the game clean, and the captured bytes are the ONLY copy
	// of the user's tuning (finding 2 — rollback used to drop them).
	iniPath := filepath.Join(e.bin, "OptiScaler.ini")
	if err := os.WriteFile(iniPath, customINIBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	// Fault injection via the gap seam: the uninstall leg just removed the
	// old build; a dangling symlink at the injection target breaks the
	// install leg's first copy mid-swap, after earlier bundle files landed.
	dxgi := filepath.Join(e.bin, "dxgi.dll")
	e.sess.upgradeGapHook = func(gameDir string) {
		if err := os.Symlink(filepath.Join(e.bin, "no-such-dir", "dxgi.dll"), dxgi); err != nil {
			t.Errorf("gap hook sabotage: %v", err)
		}
	}

	e.sess.SwitchVersion(e.gameRoot, "v0.10.0-test")
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Uninstalled") {
		t.Fatalf("first settle = %q, want the uninstall leg", ev.Text)
	}
	waitEvent(t, e.sess, EvOpFailed) // install leg failed mid-swap
	toast := waitToast(t, e.sess, "Failed:")
	if !toast.Warn {
		t.Errorf("failure toast Warn = false, want true: %+v", toast)
	}
	ev = waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Rolled back") {
		t.Fatalf("cleanup settle = %q, want the rollback/backup-restore leg", ev.Text)
	}

	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	if manifests[0].Status != domain.StatusRolledBack {
		t.Errorf("manifest status = %q, want rolled_back (no failed manifest lingers)", manifests[0].Status)
	}
	if _, err := os.Lstat(dxgi); !os.IsNotExist(err) {
		t.Errorf("dxgi.dll (symlink) survived the rollback: err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(e.bin, "fakenvapi.dll")); !os.IsNotExist(err) {
		t.Error("partial bundle file fakenvapi.dll survived the rollback")
	}
	// Finding 2: the captured ini is written back over the clean game dir
	// (orphan ini preserves the user's tuning).
	got, err := os.ReadFile(iniPath)
	if err != nil {
		t.Fatalf("OptiScaler.ini missing after failed switch + rollback: %v", err)
	}
	if !bytes.Equal(got, customINIBytes) {
		t.Errorf("ini bytes after rollback = %q, want the custom bytes", got)
	}
	if st, err := os.Stat(iniPath); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("ini mode after rollback = %v (err %v), want 0600", st.Mode().Perm(), err)
	}
	t.Log("S7: install leg failed after uninstall: error toast surfaced, rollback cleaned the half-state, ini restored")
}

// TestSwitchINIRestoreFailureKeepsInstall (S9): when the ini write-back
// fails (the ini path became a directory), the install still stands
// committed at the new version — the failure surfaces as a warning toast,
// not a rollback and not a crash.
func TestSwitchINIRestoreFailureKeepsInstall(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)

	iniPath := filepath.Join(e.bin, "OptiScaler.ini")
	if err := os.WriteFile(iniPath, customINIBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// The install leg just succeeded; sabotage the write-back by turning
	// the ini path into a directory so os.WriteFile refuses it.
	e.sess.switchINIHook = func(gameDir string) {
		if err := os.Remove(iniPath); err != nil {
			t.Errorf("ini hook remove: %v", err)
		}
		if err := os.Mkdir(iniPath, 0o755); err != nil {
			t.Errorf("ini hook mkdir: %v", err)
		}
	}

	e.sess.SwitchVersion(e.gameRoot, "v0.10.0-test")
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Uninstalled") {
		t.Fatalf("first settle = %q, want the uninstall leg", ev.Text)
	}
	ev = waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Installed") {
		t.Fatalf("second settle = %q, want the install leg (it must STAND)", ev.Text)
	}
	toast := waitToast(t, e.sess, "restoring your OptiScaler.ini failed")
	if !toast.Warn {
		t.Errorf("ini-restore toast Warn = false, want true: %+v", toast)
	}

	// The install stands committed at the chosen version.
	row := theRow(t, e.sess)
	if row.Status != domain.StatusCommitted {
		t.Fatalf("row status after failed ini restore = %q, want committed", row.Status)
	}
	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	if manifests[0].Resolved.Version != "v0.10.0-test" {
		t.Errorf("manifest version = %q, want v0.10.0-test", manifests[0].Resolved.Version)
	}
	t.Log("S9: ini restore failed; install stands at the new version, warn toast surfaced")
}

// TestSwitchSameVersionIsNoOp (S13): switching to the ALREADY installed
// version dispatches nothing — no uninstall, no install, no op events, no
// resolution, and the ini stays byte-identical.
func TestSwitchSameVersionIsNoOp(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)

	row := theRow(t, e.sess)
	installed := row.OptiScalerVersion
	if installed == "" {
		t.Fatalf("row = %+v, want an installed version", row)
	}
	iniPath := filepath.Join(e.bin, "OptiScaler.ini")
	if err := os.WriteFile(iniPath, customINIBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	// Drain events still in flight from the install, then stay silent.
	for {
		select {
		case <-e.sess.Events():
		default:
			goto drained
		}
	}
drained:
	resolvesBefore := e.resolves.Load()
	e.sess.SwitchVersion(e.gameRoot, installed)

	select {
	case ev := <-e.sess.Events():
		t.Fatalf("same-version switch dispatched %v %q, want total silence", ev.Kind, ev.Text)
	case <-time.After(300 * time.Millisecond):
	}
	if got := e.resolves.Load(); got != resolvesBefore {
		t.Errorf("resolves = %d, want %d (no resolution for a no-op switch)", got, resolvesBefore)
	}
	if e.sess.OpBusy(e.gameRoot) {
		t.Error("OpBusy true after a no-op switch: an op was registered")
	}
	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	if manifests[0].Resolved.Version != "v0.9.4-test" {
		t.Errorf("manifest version = %q, want v0.9.4-test (untouched)", manifests[0].Resolved.Version)
	}
	got, err := os.ReadFile(iniPath)
	if err != nil || !bytes.Equal(got, customINIBytes) {
		t.Errorf("ini after no-op switch = %q (err %v), want the custom bytes untouched", got, err)
	}
	if row := theRow(t, e.sess); row.Status != domain.StatusCommitted || row.OptiScalerVersion != installed {
		t.Errorf("row after no-op switch = %+v, want committed at %q", row, installed)
	}
	t.Log("S13: same-version switch dispatched nothing; state byte-identical")
}

// TestSwitchBusyGameRefuses: a game with an op in flight refuses the switch
// gracefully — the busy toast surfaces and the committed install stays
// exactly as it was (errOpBusy semantics, mirroring doUpgrade).
func TestSwitchBusyGameRefuses(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)

	if _, ok := e.sess.registerOp(e.gameRoot); !ok {
		t.Fatal("op slot not free after install settled")
	}
	t.Cleanup(func() { e.sess.finishOp(e.gameRoot) })

	e.sess.SwitchVersion(e.gameRoot, "v0.10.0-test")
	waitToast(t, e.sess, "operation already in progress for this game")

	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1 (switch refused)", len(manifests), err)
	}
	if manifests[0].Resolved.Version != "v0.9.4-test" {
		t.Errorf("manifest version = %q, want v0.9.4-test (untouched)", manifests[0].Resolved.Version)
	}
	if row := theRow(t, e.sess); row.Status != domain.StatusCommitted {
		t.Errorf("row status = %q, want committed (switch refused)", row.Status)
	}
	t.Log("busy guard: switch refused with the busy toast, old install untouched")
}

// installEACAt pins an EAC-flagged game at an installed version: the scan
// flags the row, the install-time EAC gate is consented once, and the game
// settles committed at the fixture's older version.
func installEACAt(t *testing.T, e *upgradeEnv) {
	t.Helper()
	writeUIFile(t, filepath.Join(e.gameRoot, "start_protected_game.exe"), "EAC")
	scanAndWait(t, e.sess)
	if row := theRow(t, e.sess); !row.EAC {
		t.Fatalf("row EAC = false, want true (exe marker not detected)")
	}
	e.sess.QuickInstall(e.gameRoot)
	waitEvent(t, e.sess, EvConfirm)
	e.sess.AnswerConfirm(true)
	waitEvent(t, e.sess, EvOpDone)
	if row := theRow(t, e.sess); row.Status != domain.StatusCommitted {
		t.Fatalf("row status = %q after consented install, want committed", row.Status)
	}
}

// TestSwitchEACCommittedConsentAccepts (finding 1a): switching an
// EAC-flagged committed game asks for consent BEFORE anything destructive —
// the custom ini and the old build are provably untouched while the prompt
// is pending — and accepting runs the full switch chain at the chosen tag
// with the ini (bytes AND mode) intact.
func TestSwitchEACCommittedConsentAccepts(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installEACAt(t, e)

	iniPath := filepath.Join(e.bin, "OptiScaler.ini")
	if err := os.WriteFile(iniPath, customINIBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	e.sess.SwitchVersion(e.gameRoot, "v0.10.0-test")
	waitEvent(t, e.sess, EvConfirm)
	st := e.sess.Snapshot()
	if st.Confirm == nil || st.Confirm.Kind != ConfirmVersionSwitch {
		t.Fatalf("pending confirmation = %+v, want Kind ConfirmVersionSwitch", st.Confirm)
	}
	if st.Confirm.Version != "v0.10.0-test" {
		t.Errorf("confirmation version = %q, want the chosen tag v0.10.0-test", st.Confirm.Version)
	}
	if !strings.Contains(st.Confirm.Message, "Easy Anti-Cheat") {
		t.Errorf("confirmation message = %q, want the shared EAC warning", st.Confirm.Message)
	}
	// Pre-flight: nothing destructive ran while consent is pending.
	if got, err := os.ReadFile(iniPath); err != nil || !bytes.Equal(got, customINIBytes) {
		t.Fatalf("ini touched before consent: %q (err %v)", got, err)
	}
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Fatalf("old build uninstalled before consent: %v", err)
	}

	e.sess.AnswerConfirm(true)
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Uninstalled") {
		t.Fatalf("first settle = %q, want the uninstall leg", ev.Text)
	}
	ev = waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Installed") {
		t.Fatalf("second settle = %q, want the install leg", ev.Text)
	}

	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	if manifests[0].Resolved.Version != "v0.10.0-test" {
		t.Errorf("manifest version = %q, want v0.10.0-test", manifests[0].Resolved.Version)
	}
	got, err := os.ReadFile(iniPath)
	if err != nil || !bytes.Equal(got, customINIBytes) {
		t.Errorf("ini after consented switch = %q (err %v), want the custom bytes", got, err)
	}
	if st, err := os.Stat(iniPath); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("ini mode after consented switch = %v (err %v), want 0600", st.Mode().Perm(), err)
	}
	t.Log("1a: EAC pre-flight consented; full switch chain at the chosen tag, ini intact")
}

// TestSwitchEACCommittedConsentDeclined (finding 1b): declining the
// switch consent leaves EVERYTHING byte-identical — the ini on disk, the
// committed old build, the manifest — and no operation ever ran.
func TestSwitchEACCommittedConsentDeclined(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installEACAt(t, e)

	iniPath := filepath.Join(e.bin, "OptiScaler.ini")
	if err := os.WriteFile(iniPath, customINIBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	e.sess.SwitchVersion(e.gameRoot, "v0.10.0-test")
	waitEvent(t, e.sess, EvConfirm)
	e.sess.AnswerConfirm(false)
	if st := e.sess.Snapshot(); st.Confirm != nil {
		t.Fatalf("declined confirmation not cleared: %+v", st.Confirm)
	}

	select {
	case ev := <-e.sess.Events():
		t.Fatalf("declined switch dispatched %v %q, want total silence", ev.Kind, ev.Text)
	case <-time.After(300 * time.Millisecond):
	}

	got, err := os.ReadFile(iniPath)
	if err != nil || !bytes.Equal(got, customINIBytes) {
		t.Errorf("ini after declined switch = %q (err %v), want the custom bytes untouched", got, err)
	}
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Errorf("old build removed after declined switch: %v", err)
	}
	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	if manifests[0].Resolved.Version != "v0.9.4-test" {
		t.Errorf("manifest version = %q, want v0.9.4-test (untouched)", manifests[0].Resolved.Version)
	}
	if row := theRow(t, e.sess); row.Status != domain.StatusCommitted {
		t.Errorf("row status = %q, want committed (decline = zero operation)", row.Status)
	}
	t.Log("1b: EAC switch declined; ini and old build byte-identical, no op ran")
}

// TestSwitchEACExternalConsentAccepts (finding 1, external branch): an
// EAC-flagged EXTERNAL game hits the same pre-flight; accepting
// adopt-installs at the chosen tag with the external ini preserved.
func TestSwitchEACExternalConsentAccepts(t *testing.T) {
	e := newUpgradeEnv(t, "latest")
	writeUIFile(t, filepath.Join(e.gameRoot, "start_protected_game.exe"), "EAC")
	writeExternalMarker(t, e.bin)
	writeUIFile(t, filepath.Join(e.bin, "OptiScaler.ini"), string(customINIBytes))

	scanAndWait(t, e.sess)
	row := theRow(t, e.sess)
	if row.Status != domain.StatusExternal {
		t.Fatalf("row status = %q, want external", row.Status)
	}
	if !row.EAC {
		t.Fatalf("row EAC = false, want true")
	}

	e.sess.SwitchVersion(e.gameRoot, "v0.9.4-test")
	waitEvent(t, e.sess, EvConfirm)
	st := e.sess.Snapshot()
	if st.Confirm == nil || st.Confirm.Kind != ConfirmVersionSwitch {
		t.Fatalf("pending confirmation = %+v, want Kind ConfirmVersionSwitch", st.Confirm)
	}
	if got, err := os.ReadFile(filepath.Join(e.bin, "OptiScaler.ini")); err != nil ||
		!bytes.Equal(got, customINIBytes) {
		t.Fatalf("ini touched before consent: %q (err %v)", got, err)
	}

	e.sess.AnswerConfirm(true)
	ev := waitEvent(t, e.sess, EvOpDone)
	if !strings.Contains(ev.Text, "Installed") {
		t.Fatalf("settle event = %q, want the adopt install", ev.Text)
	}

	row = theRow(t, e.sess)
	if row.Status != domain.StatusCommitted {
		t.Fatalf("row status after consented adopt-switch = %q, want committed", row.Status)
	}
	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1", len(manifests), err)
	}
	if manifests[0].Resolved.Version != "v0.9.4-test" {
		t.Errorf("manifest version = %q, want v0.9.4-test (the chosen tag)", manifests[0].Resolved.Version)
	}
	got, err := os.ReadFile(filepath.Join(e.bin, "OptiScaler.ini"))
	if err != nil || !bytes.Equal(got, customINIBytes) {
		t.Errorf("ini after consented adopt-switch = %q (err %v), want the custom bytes", got, err)
	}
	t.Log("external EAC: pre-flight consented; adopt-install at the chosen tag, ini preserved")
}

// TestSwitchINIBackupIsCrashDurable (finding 3): between the ini's removal
// and its write-back the bytes must survive an app crash — a copy under
// the session data root EXISTS mid-switch (asserted from the gap seam)
// and is deleted once the ini is safely restored.
func TestSwitchINIBackupIsCrashDurable(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	e.sess.deps.SettingsRoot = t.TempDir()
	installAt(t, e)

	iniPath := filepath.Join(e.bin, "OptiScaler.ini")
	if err := os.WriteFile(iniPath, customINIBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	backupGlob := filepath.Join(e.sess.deps.SettingsRoot, "ini-switch-backup-*.tmp")
	e.sess.upgradeGapHook = func(gameDir string) {
		matches, err := filepath.Glob(backupGlob)
		if err != nil || len(matches) != 1 {
			t.Errorf("mid-switch crash backups = %v (err %v), want exactly 1", matches, err)
			return
		}
		data, err := os.ReadFile(matches[0])
		if err != nil || !bytes.Equal(data, customINIBytes) {
			t.Errorf("mid-switch backup bytes = %q (err %v), want the custom ini", data, err)
		}
	}

	e.sess.SwitchVersion(e.gameRoot, "v0.10.0-test")
	waitEvent(t, e.sess, EvOpDone) // uninstall leg
	waitEvent(t, e.sess, EvOpDone) // install leg

	if matches, err := filepath.Glob(backupGlob); err != nil || len(matches) != 0 {
		t.Errorf("crash backup after completed switch = %v (err %v), want cleaned up", matches, err)
	}
	if got, err := os.ReadFile(iniPath); err != nil || !bytes.Equal(got, customINIBytes) {
		t.Errorf("ini after switch = %q (err %v), want the custom bytes restored", got, err)
	}
	t.Log("3: crash backup existed mid-switch with the ini bytes, gone after restore")
}

// TestSwitchAbortsWhenINIBackupFails (finding 3, refusal path): when the
// crash-durable copy cannot be written, the switch aborts BEFORE anything
// destructive — warn toast, ini untouched, old build untouched.
func TestSwitchAbortsWhenINIBackupFails(t *testing.T) {
	e := newUpgradeEnv(t, "v0.9.4-test")
	installAt(t, e)

	// A data root that is a regular FILE makes the backup write fail.
	rootFile := filepath.Join(t.TempDir(), "not-a-dir")
	writeUIFile(t, rootFile, "x")
	e.sess.deps.SettingsRoot = rootFile

	iniPath := filepath.Join(e.bin, "OptiScaler.ini")
	if err := os.WriteFile(iniPath, customINIBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	e.sess.SwitchVersion(e.gameRoot, "v0.10.0-test")
	toast := waitToast(t, e.sess, "back up")
	if !toast.Warn {
		t.Errorf("backup-failure toast Warn = false, want true: %+v", toast)
	}

	got, err := os.ReadFile(iniPath)
	if err != nil || !bytes.Equal(got, customINIBytes) {
		t.Errorf("ini after aborted switch = %q (err %v), want the custom bytes untouched", got, err)
	}
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Errorf("old build removed after aborted switch: %v", err)
	}
	manifests, err := e.store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("manifests = %d, err %v; want 1 (switch aborted)", len(manifests), err)
	}
	if manifests[0].Resolved.Version != "v0.9.4-test" {
		t.Errorf("manifest version = %q, want v0.9.4-test (untouched)", manifests[0].Resolved.Version)
	}
	t.Log("3b: backup write failed; switch aborted before any destructive step")
}
