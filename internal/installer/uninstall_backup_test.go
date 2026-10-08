package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// loseBackup deletes one adopt-time backup entry, simulating the
// semi-manual/fork-switch scenario from issue 050 where the
// optiscaler-backups store is incomplete at uninstall time.
func loseBackup(t *testing.T, bin, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(bin, "optiscaler-backups", "files", name)); err != nil {
		t.Fatalf("lose backup %s: %v", name, err)
	}
}

// TestUninstallMissingBackupTypedError: a missing backup must not abort
// the uninstall with an opaque error — a typed MissingBackupError lists
// EVERY unrestorable file, progress made so far is persisted, and the
// install stays committed so the user can decide (issue 050).
func TestUninstallMissingBackupTypedError(t *testing.T) {
	root, bin, st := newGame(t)
	writeFile(t, filepath.Join(bin, "dxgi.dll"), "ORIGINAL-DXGI")
	writeFile(t, filepath.Join(bin, "OptiScaler.ini"), "ORIGINAL-INI")
	if _, err := Install(context.Background(), st, request(root, bin)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	id := manifestID(t, bin)
	loseBackup(t, bin, "dxgi.dll")
	loseBackup(t, bin, "OptiScaler.ini")

	err := Uninstall(context.Background(), st, id)
	var mb *MissingBackupError
	if !errors.As(err, &mb) {
		t.Fatalf("Uninstall err = %v, want MissingBackupError", err)
	}
	if len(mb.Paths) != 2 {
		t.Fatalf("MissingBackupError.Paths = %v, want both lost originals", mb.Paths)
	}
	for _, p := range mb.Paths {
		if !strings.Contains(p, "dxgi.dll") && !strings.Contains(p, "OptiScaler.ini") {
			t.Errorf("unexpected path in MissingBackupError: %s", p)
		}
	}

	// Nothing unrestorable was half-removed: both mod files still in place,
	// manifest still committed with both overwritten entries retained.
	for _, name := range []string{"dxgi.dll", "OptiScaler.ini"} {
		if _, serr := os.Stat(filepath.Join(bin, name)); serr != nil {
			t.Errorf("%s vanished despite declined uninstall: %v", name, serr)
		}
	}
	m, lerr := st.Load(id)
	if lerr != nil {
		t.Fatalf("manifest missing after blocked uninstall: %v", lerr)
	}
	if m.Status != domain.StatusCommitted {
		t.Errorf("status %q, want committed", m.Status)
	}
	if len(m.Overwritten) != 2 {
		t.Errorf("overwritten = %+v, want both entries retained", m.Overwritten)
	}
	t.Logf("typed error names all lost backups: %v", mb)
}

// TestUninstallAllowMissingBackupDropsFile: with the user's explicit
// consent the uninstall proceeds — the mod's bytes are removed, the lost
// original is simply not restored, and the install tears down completely
// (issue 050).
func TestUninstallAllowMissingBackupDropsFile(t *testing.T) {
	root, bin, st := newGame(t)
	writeFile(t, filepath.Join(bin, "dxgi.dll"), "ORIGINAL-DXGI")
	if _, err := Install(context.Background(), st, request(root, bin)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	id := manifestID(t, bin)
	loseBackup(t, bin, "dxgi.dll")

	if err := UninstallWithOptions(context.Background(), st, id,
		UninstallOptions{AllowMissingBackup: true}); err != nil {
		t.Fatalf("UninstallWithOptions(AllowMissingBackup): %v", err)
	}
	if _, err := os.Stat(filepath.Join(bin, "dxgi.dll")); !os.IsNotExist(err) {
		t.Error("mod dxgi.dll survived consented removal")
	}
	if _, err := st.Load(id); err == nil {
		t.Error("manifest should be deleted after consented removal")
	}
	if _, err := os.Stat(filepath.Join(bin, "optiscaler-backups")); !os.IsNotExist(err) {
		t.Error("backup dir should be deleted after consented removal")
	}
	t.Log("missing backup bypassed by consent: mod removed, manifest deleted")
}

// TestUninstallMissingBackupVanishedFile: the already-gone installed file
// with an already-gone backup is a no-op even WITHOUT consent — nothing
// exists to remove or restore, so it must not block (issue 050).
func TestUninstallMissingBackupVanishedFile(t *testing.T) {
	root, bin, st := newGame(t)
	writeFile(t, filepath.Join(bin, "dxgi.dll"), "ORIGINAL-DXGI")
	if _, err := Install(context.Background(), st, request(root, bin)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	id := manifestID(t, bin)
	loseBackup(t, bin, "dxgi.dll")
	if err := os.Remove(filepath.Join(bin, "dxgi.dll")); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(context.Background(), st, id); err != nil {
		t.Fatalf("Uninstall with vanished file+backup should succeed: %v", err)
	}
	t.Log("vanished file with vanished backup uninstalled cleanly")
}
