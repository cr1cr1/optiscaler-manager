package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// assertCleanGameDir fails the test unless the install dir holds exactly the
// files it held before the op (byte-for-byte, no extras).
func assertCleanGameDir(t *testing.T, gameRoot, before string) {
	t.Helper()
	after := snapshot(t, gameRoot)
	if before != after {
		t.Errorf("game dir not restored byte-for-byte\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestInstallCancelBeforeExtract_RollsBack seeds a leftover failed install
// with partial game-dir state, then calls Install with an already-cancelled
// context. The leftover must be rolled back to completion (rollback is
// cleanup, not op work), and Install must report context.Canceled before any
// extract/write of its own.
func TestInstallCancelBeforeExtract_RollsBack(t *testing.T) {
	root, bin, st := newGame(t)
	original := "ORIGINAL-DXGI"
	writeFile(t, filepath.Join(bin, "dxgi.dll"), original)
	before := snapshot(t, root)

	// Seed a leftover failed install with partial state on disk.
	calls := 0
	orig := copyFileFn
	copyFileFn = func(src, dst string) (string, error) {
		calls++
		if calls == 2 {
			return "", fmt.Errorf("injected copy failure on %s", dst)
		}
		return orig(src, dst)
	}
	if _, err := Install(context.Background(), st, request(root, bin)); err == nil {
		t.Fatal("expected injected failure seeding the leftover")
	}
	copyFileFn = orig
	id := manifestID(t, bin)
	if m, _ := st.Load(id); m.Status != domain.StatusFailed {
		t.Fatalf("leftover status %q, want failed", m.Status)
	}
	t.Logf("leftover failed install seeded after %d copies", calls)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Install(ctx, st, request(root, bin))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Install err = %v, want errors.Is context.Canceled", err)
	}

	assertCleanGameDir(t, root, before)
	if data, _ := os.ReadFile(filepath.Join(bin, "dxgi.dll")); string(data) != original {
		t.Error("original dxgi.dll not restored by cancel-boundary rollback")
	}
	m, lerr := st.Load(id)
	if lerr != nil {
		t.Fatalf("manifest missing: %v", lerr)
	}
	if m.Status != domain.StatusRolledBack {
		t.Errorf("leftover status %q, want rolled_back", m.Status)
	}
	t.Logf("cancel before extract: leftover rolled back, FS clean, err=%v", err)
}

// TestInstallCancelMidSwap_ManifestFailedAndRollbackClean cancels the context
// from inside the file-swap loop. The manifest must record the failure
// (last_error names the cancellation), the automatic rollback must restore
// the pre-op state with zero partial files, and the error must unwrap to
// context.Canceled.
func TestInstallCancelMidSwap_ManifestFailedAndRollbackClean(t *testing.T) {
	root, bin, st := newGame(t)
	original := "ORIGINAL-DXGI"
	writeFile(t, filepath.Join(bin, "dxgi.dll"), original)
	before := snapshot(t, root)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Complete the first install write (call 2: call 1 is the backup copy of
	// the pre-existing dxgi.dll), then cancel mid-swap.
	calls := 0
	orig := copyFileFn
	copyFileFn = func(src, dst string) (string, error) {
		calls++
		sha, err := orig(src, dst)
		if calls == 2 {
			cancel()
		}
		return sha, err
	}
	defer func() { copyFileFn = orig }()

	_, err := Install(ctx, st, request(root, bin))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Install err = %v, want errors.Is context.Canceled", err)
	}

	id := manifestID(t, bin)
	m, lerr := st.Load(id)
	if lerr != nil {
		t.Fatalf("manifest missing after cancelled install: %v", lerr)
	}
	// failed is recorded (last_error), then the automatic rollback moved the
	// manifest to rolled_back — the failure must remain visible.
	if !strings.Contains(m.LastError, context.Canceled.Error()) {
		t.Errorf("LastError %q does not record the cancellation", m.LastError)
	}
	if m.Status != domain.StatusRolledBack {
		t.Errorf("status %q after cancel, want rolled_back (failed → rolled_back)", m.Status)
	}

	assertCleanGameDir(t, root, before)
	if data, _ := os.ReadFile(filepath.Join(bin, "dxgi.dll")); string(data) != original {
		t.Error("original dxgi.dll not restored after mid-swap cancel")
	}
	entries, _ := os.ReadDir(bin)
	for _, e := range entries {
		t.Logf("remaining in bin: %s", e.Name())
	}
	if _, err := os.Stat(filepath.Join(bin, "fakenvapi.dll")); !os.IsNotExist(err) {
		t.Error("partial created file fakenvapi.dll survived cancel")
	}
	t.Logf("cancel mid-swap after %d copies: manifest failed+rolled back, FS clean", calls)
}

// TestUninstallCancel_IdempotentResume cancels mid-uninstall. The manifest
// must stay committed with progress persisted (processed entries dropped),
// and a retry with a live context must resume and complete byte-cleanly.
func TestUninstallCancel_IdempotentResume(t *testing.T) {
	root, bin, st := newGame(t)
	origDxgi := "ORIGINAL-DXGI"
	origIni := "ORIGINAL-INI"
	writeFile(t, filepath.Join(bin, "dxgi.dll"), origDxgi)
	writeFile(t, filepath.Join(bin, "OptiScaler.ini"), origIni)
	before := snapshot(t, root)

	if _, err := Install(context.Background(), st, request(root, bin)); err != nil {
		t.Fatalf("Install: %v", err)
	}
	id := manifestID(t, bin)

	// Cancel from inside the first overwrite-restore copy.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	restores := 0
	orig := copyFileFn
	copyFileFn = func(src, dst string) (string, error) {
		restores++
		sha, err := orig(src, dst)
		cancel()
		return sha, err
	}

	err := Uninstall(ctx, st, id)
	copyFileFn = orig
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Uninstall err = %v, want errors.Is context.Canceled", err)
	}
	if restores != 1 {
		t.Fatalf("expected exactly 1 restore before cancel took effect, got %d", restores)
	}

	// Progress persisted: manifest still committed, the restored entry
	// dropped, unprocessed entries retained.
	m, lerr := st.Load(id)
	if lerr != nil {
		t.Fatalf("manifest missing after cancelled uninstall: %v", lerr)
	}
	if m.Status != domain.StatusCommitted {
		t.Errorf("status %q after cancel, want committed", m.Status)
	}
	if len(m.Overwritten) != 1 || filepath.Base(m.Overwritten[0].Path) != "OptiScaler.ini" {
		t.Errorf("overwritten progress = %+v, want only OptiScaler.ini retained", m.Overwritten)
	}
	if data, _ := os.ReadFile(filepath.Join(bin, "dxgi.dll")); string(data) != origDxgi {
		t.Error("dxgi.dll not restored before cancel took effect")
	}
	t.Logf("cancel mid-uninstall: committed manifest retained %+v overwritten", m.Overwritten)

	// Resume with a live context: must complete and restore byte-for-byte.
	if err := Uninstall(context.Background(), st, id); err != nil {
		t.Fatalf("resumed Uninstall: %v", err)
	}
	assertCleanGameDir(t, root, before)
	if _, err := st.Load(id); err == nil {
		t.Error("manifest should be deleted after resumed clean uninstall")
	}
	if _, err := os.Stat(st.BackupDir(id)); !os.IsNotExist(err) {
		t.Error("backup dir should be deleted after resumed clean uninstall")
	}
	t.Log("cancelled uninstall resumed idempotently to a byte-clean state")
}
