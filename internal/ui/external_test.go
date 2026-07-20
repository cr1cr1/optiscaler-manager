package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// notManagedRefusal is the exact clean-toast substring required when an
// uninstall targets an install this manager never made.
const notManagedRefusal = "not installed by this manager — adopt first or remove manually"

// writeExternalMarker plants a synthetic OptiScaler-branded dxgi.dll (an
// "external" install: present on disk, no manager manifest) and returns its
// bytes so round-trip tests can verify restoration.
func writeExternalMarker(t *testing.T, dir string) []byte {
	t.Helper()
	marker := testutil.StringInfoPE(false, map[string]string{
		"ProductName":      "OptiScaler",
		"OriginalFilename": "OptiScaler.dll",
	}, [4]uint16{0, 7, 0, 0})
	writeUIFile(t, filepath.Join(dir, "dxgi.dll"), string(marker))
	return marker
}

// scanExternalRow scans the fixture library with an external marker planted
// and returns the resulting row, which must be Status external.
func scanExternalRow(t *testing.T, e *testEnv) GameRow {
	t.Helper()
	writeExternalMarker(t, e.bin)
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	rows := e.sess.Snapshot().Rows
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Status != domain.StatusExternal {
		t.Fatalf("row status = %q, want %q (marker dxgi.dll undetected)", rows[0].Status, domain.StatusExternal)
	}
	return rows[0]
}

// waitToast polls until a toast containing substr appears or the deadline.
func waitToast(t *testing.T, s *Session, substr string) Toast {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		for _, toast := range s.Snapshot().Toasts {
			if strings.Contains(toast.Text, substr) {
				return toast
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for toast containing %q; toasts: %+v", substr, s.Snapshot().Toasts)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// assertNoRawFailureToast fails when any toast leaks a raw store/op error
// instead of the clean refusal.
func assertNoRawFailureToast(t *testing.T, s *Session) {
	t.Helper()
	for _, toast := range s.Snapshot().Toasts {
		if strings.HasPrefix(toast.Text, "Failed:") {
			t.Errorf("raw failure toast leaked: %q", toast.Text)
		}
	}
}

// TestUninstallExternalRefused: an external row was never installed by this
// manager, so Uninstall refuses up front with a clean toast — no op is
// registered, the store stays untouched, and no raw store error leaks.
func TestUninstallExternalRefused(t *testing.T) {
	e := newTestEnv(t)
	row := scanExternalRow(t, e)

	e.sess.Uninstall(row.InstallDir)
	toast := waitToast(t, e.sess, notManagedRefusal)
	if !toast.Warn {
		t.Errorf("refusal toast Warn = false, want true: %+v", toast)
	}
	if e.sess.OpBusy(row.InstallDir) {
		t.Error("OpBusy true after refusal: an op was registered for an external row")
	}
	manifests, err := e.sess.deps.Store.List()
	if err != nil || len(manifests) != 0 {
		t.Errorf("store touched by refused uninstall: manifests=%d err=%v", len(manifests), err)
	}
	assertNoRawFailureToast(t, e.sess)
	// The external files must be exactly as found: nothing removed.
	if _, err := os.Stat(filepath.Join(e.bin, "dxgi.dll")); err != nil {
		t.Errorf("external dxgi.dll vanished after refused uninstall: %v", err)
	}
}

// TestDoUninstallNotManagedCleanToast: when the store reports ErrNotManaged
// (manifest vanished between scan and op), the raw sentinel must never reach
// the user — the same clean refusal toast surfaces instead.
func TestDoUninstallNotManagedCleanToast(t *testing.T) {
	e := newTestEnv(t)
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	row := e.sess.Snapshot().Rows[0]
	if row.Status != "" {
		t.Fatalf("row status = %q, want uninstalled", row.Status)
	}

	e.sess.Uninstall(row.InstallDir)
	toast := waitToast(t, e.sess, notManagedRefusal)
	if !toast.Warn {
		t.Errorf("refusal toast Warn = false, want true: %+v", toast)
	}
	if e.sess.OpBusy(row.InstallDir) {
		t.Error("OpBusy true after the op settled")
	}
	assertNoRawFailureToast(t, e.sess)
}

// TestReconcileKeepsCachedExternalRow: a warm games cache holding an
// external row must survive Start's manifest-based reconcile — manifests
// override only where they exist, so an unmanaged row keeps its derived
// external status instead of dropping to "".
func TestReconcileKeepsCachedExternalRow(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	saveGamesCache(e.sess.deps.SettingsRoot, []GameRow{{
		Title:        "Game One",
		AppID:        "100",
		InstallDir:   e.gameRoot,
		InjectionDir: e.bin,
		Platform:     domain.StoreSteam.String(),
		Store:        domain.StoreSteam,
		Status:       domain.StatusExternal,
	}})

	e.sess.Start(context.Background()) // warm cache: reconciles, never scans
	st := e.sess.Snapshot()
	if st.StatusLine != "1 games (cached)" {
		t.Fatalf("StatusLine = %q, want warm-cache boot (no scan)", st.StatusLine)
	}
	if len(st.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(st.Rows))
	}
	if st.Rows[0].Status != domain.StatusExternal {
		t.Errorf("cached external row reconciled to %q, want %q", st.Rows[0].Status, domain.StatusExternal)
	}
}

// TestAdoptRoundTripRestoresExternalBytes is the keystone: an external
// OptiScaler install (marker dxgi.dll, no manifest) is adopted by
// QuickInstall — which must back it up — then uninstalled, which must
// restore the exact marker bytes from the SHA-verified backup, and the
// post-uninstall re-detect must surface the row as external again.
func TestAdoptRoundTripRestoresExternalBytes(t *testing.T) {
	e := newTestEnv(t)
	marker := writeExternalMarker(t, e.bin)
	markerSHA := sha256.Sum256(marker)
	dxgi := filepath.Join(e.bin, "dxgi.dll")
	t.Logf("marker: %d bytes sha256=%s", len(marker), hex.EncodeToString(markerSHA[:]))

	// 1. Scan → row Status external (PE detection inside the scan pipeline).
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	row := e.sess.Snapshot().Rows[0]
	if row.Status != domain.StatusExternal {
		t.Fatalf("(1) row status = %q, want %q", row.Status, domain.StatusExternal)
	}
	t.Logf("(1) scan: %q detected as external (version %q)", row.Title, row.OptiScalerVersion)

	// 2. QuickInstall on the external row adopts it: status committed, the
	// store gains a manifest, and dxgi.dll is now the bundle's file.
	e.sess.QuickInstall(row.InstallDir)
	waitEvent(t, e.sess, EvOpDone)
	row = e.sess.Snapshot().Rows[0]
	if row.Status != domain.StatusCommitted {
		t.Fatalf("(2) row status = %q after adopt, want %q", row.Status, domain.StatusCommitted)
	}
	manifests, err := e.sess.deps.Store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("(2) adopt left %d manifests (err %v), want 1", len(manifests), err)
	}
	adopted, err := os.ReadFile(dxgi)
	if err != nil {
		t.Fatalf("(2) dxgi.dll missing after adopt: %v", err)
	}
	adoptedSHA := sha256.Sum256(adopted)
	if adoptedSHA == markerSHA {
		t.Fatal("(2) adopt did not replace the external dxgi.dll with the bundle's")
	}
	t.Logf("(2) adopt: committed, manifest %s, dxgi.dll sha256=%s (!= marker)",
		manifests[0].ID, hex.EncodeToString(adoptedSHA[:]))

	// 3. Uninstall succeeds: the row is managed now, so no refusal.
	e.sess.Uninstall(row.InstallDir)
	waitEvent(t, e.sess, EvOpDone)

	// 4. The SHA-verified backup restored the exact marker bytes.
	restored, err := os.ReadFile(dxgi)
	if err != nil {
		t.Fatalf("(4) external dxgi.dll not restored by uninstall: %v", err)
	}
	restoredSHA := sha256.Sum256(restored)
	if restoredSHA != markerSHA {
		t.Fatalf("(4) restored dxgi.dll sha256=%s, want marker %s",
			hex.EncodeToString(restoredSHA[:]), hex.EncodeToString(markerSHA[:]))
	}
	t.Logf("(4) uninstall restored marker bytes: sha256=%s == marker", hex.EncodeToString(restoredSHA[:]))

	// 5. Post-uninstall re-detect: the restored external install shows as
	// external again, not as a bare uninstalled game.
	row = e.sess.Snapshot().Rows[0]
	if row.Status != domain.StatusExternal {
		t.Fatalf("(5) row status = %q after uninstall restored the external install, want %q",
			row.Status, domain.StatusExternal)
	}
	t.Logf("(5) row external again — round trip closed")
}

// TestAdoptFailRollbackReturnsExternal is the keystone rollback leg: an
// external OptiScaler install (marker dxgi.dll) is adopted, the install
// fails mid-swap (a directory blocks the fakenvapi.dll destination after
// the external dxgi.dll was backed up), and Rollback must restore the exact
// marker bytes AND surface the row as external again — not rolled_back,
// because the disk once more holds a working external install.
func TestAdoptFailRollbackReturnsExternal(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir() // warm-cache leg needs persistence
	marker := writeExternalMarker(t, e.bin)
	markerSHA := sha256.Sum256(marker)
	dxgi := filepath.Join(e.bin, "dxgi.dll")

	// 1. Scan → row Status external.
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	row := e.sess.Snapshot().Rows[0]
	if row.Status != domain.StatusExternal {
		t.Fatalf("(1) row status = %q, want %q", row.Status, domain.StatusExternal)
	}

	// 2. Fault injection: a directory named fakenvapi.dll blocks that bundle
	// destination. The plan copies OptiScaler.dll → dxgi.dll first (backing
	// the marker up SHA-verified), so the swap fails with a failed manifest
	// and a restorable backup.
	if err := os.MkdirAll(filepath.Join(e.bin, "fakenvapi.dll"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.sess.QuickInstall(row.InstallDir)
	waitEvent(t, e.sess, EvOpFailed)
	manifests, err := e.sess.deps.Store.List()
	if err != nil || len(manifests) != 1 {
		t.Fatalf("(2) failed adopt left %d manifests (err %v), want 1", len(manifests), err)
	}
	if manifests[0].Status != domain.StatusFailed {
		t.Fatalf("(2) manifest status = %q, want %q", manifests[0].Status, domain.StatusFailed)
	}
	swapped, err := os.ReadFile(dxgi)
	if err != nil {
		t.Fatalf("(2) dxgi.dll missing mid-failure: %v", err)
	}
	if sha256.Sum256(swapped) == markerSHA {
		t.Fatal("(2) fault injection did not fire: dxgi.dll still holds the marker")
	}
	t.Logf("(2) adopt failed mid-swap: manifest failed, dxgi.dll holds bundle bytes")

	// 3. Rollback restores the SHA-verified backup: exact marker bytes.
	e.sess.Rollback(row.InstallDir)
	waitEvent(t, e.sess, EvOpDone)
	restored, err := os.ReadFile(dxgi)
	if err != nil {
		t.Fatalf("(3) external dxgi.dll not restored by rollback: %v", err)
	}
	if got := sha256.Sum256(restored); got != markerSHA {
		t.Fatalf("(3) restored dxgi.dll sha256=%s, want marker %s",
			hex.EncodeToString(got[:]), hex.EncodeToString(markerSHA[:]))
	}

	// 4. Post-rollback re-detect: the restored external install shows as
	// external, not as a bare rolled_back row.
	row = e.sess.Snapshot().Rows[0]
	if row.Status != domain.StatusExternal {
		t.Fatalf("(4) row status = %q after rollback restored the external install, want %q",
			row.Status, domain.StatusExternal)
	}
	t.Log("(4) rollback restored marker bytes and the row is external again")

	// 5. Rescan: the rollback's idempotent job is done, so no rolled_back
	// manifest may pin the row — the enrich probe must keep it external.
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	row = e.sess.Snapshot().Rows[0]
	if row.Status != domain.StatusExternal {
		t.Fatalf("(5) row status = %q after rescan, want %q (rolled_back manifest still pinning the row)",
			row.Status, domain.StatusExternal)
	}
	t.Log("(5) rescan keeps the row external")

	// 6. Warm-cache restart: manifest-based reconcile must keep external.
	e.sess.Start(context.Background())
	st := e.sess.Snapshot()
	if st.StatusLine != "1 games (cached)" {
		t.Fatalf("(6) StatusLine = %q, want warm-cache boot", st.StatusLine)
	}
	if len(st.Rows) != 1 || st.Rows[0].Status != domain.StatusExternal {
		t.Fatalf("(6) warm-cache row status = %+v, want %q", st.Rows, domain.StatusExternal)
	}
	t.Log("(6) warm-cache reconcile keeps the row external")
}

// TestRollbackNotManagedCleanToast: when Rollback targets a game the store
// has no manifest for, the raw ErrNotManaged sentinel must never reach the
// user — the same clean refusal toast as uninstall surfaces instead.
func TestRollbackNotManagedCleanToast(t *testing.T) {
	e := newTestEnv(t)
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	row := e.sess.Snapshot().Rows[0]
	if row.Status != "" {
		t.Fatalf("row status = %q, want uninstalled", row.Status)
	}

	e.sess.Rollback(row.InstallDir)
	toast := waitToast(t, e.sess, notManagedRefusal)
	if !toast.Warn {
		t.Errorf("refusal toast Warn = false, want true: %+v", toast)
	}
	if e.sess.OpBusy(row.InstallDir) {
		t.Error("OpBusy true after the op settled")
	}
	assertNoRawFailureToast(t, e.sess)
}

// TestAddDirectoryShowsExternalImmediately: a game root added directly via
// AddDirectory (bypassing discovery→enrich) must still surface its external
// OptiScaler install as soon as the "directory added" event settles.
func TestAddDirectoryShowsExternalImmediately(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	custom := filepath.Join(t.TempDir(), "ExternalGame")
	writeUIFile(t, filepath.Join(custom, "game.exe"), "GAME")
	writeExternalMarker(t, custom)

	e.sess.AddDirectory(custom)
	waitEventText(t, e.sess, EvScanDone, "directory added")

	var row *GameRow
	rows := e.sess.Snapshot().Rows
	for i := range rows {
		if rows[i].InstallDir == custom {
			row = &rows[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("added row missing: %+v", rows)
	}
	if row.Status != domain.StatusExternal {
		t.Fatalf("row status = %q after directory add, want %q", row.Status, domain.StatusExternal)
	}
	if row.OptiScalerVersion == "" {
		t.Error("OptiScalerVersion empty on the external row")
	}
	t.Logf("manual add surfaced external immediately (version %q)", row.OptiScalerVersion)
}

// TestManagedManualGameSurvivesRescan: a game the MANAGER installed into a
// manually added folder must stay committed across rescans (manifest
// precedence — the branded OptiScaler PE on disk is the manager's own
// install, not an external one), and uninstall of it must not be refused.
func TestManagedManualGameSurvivesRescan(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	custom := filepath.Join(t.TempDir(), "ManagedManual")
	writeUIFile(t, filepath.Join(custom, "game.exe"), "GAME")
	findRow := func() *GameRow {
		rows := e.sess.Snapshot().Rows
		for i := range rows {
			if rows[i].InstallDir == custom {
				return &rows[i]
			}
		}
		return nil
	}

	e.sess.AddDirectory(custom)
	waitEventText(t, e.sess, EvScanDone, "directory added")

	// 1. Install (adopt-free, plain game) → committed.
	e.sess.QuickInstall(custom)
	waitEvent(t, e.sess, EvOpDone)
	row := findRow()
	if row == nil || row.Status != domain.StatusCommitted {
		t.Fatalf("(1) row after install = %+v, want committed", row)
	}

	// 2. Production branding: the manager-installed OptiScaler.dll carries
	// OptiScaler PE identity (the fake bundle's does not — simulate it).
	marker := testutil.StringInfoPE(false, map[string]string{
		"ProductName":      "OptiScaler",
		"OriginalFilename": "OptiScaler.dll",
	}, [4]uint16{0, 9, 4, 0})
	writeUIFile(t, filepath.Join(custom, "OptiScaler.dll"), string(marker))

	// 3. Rescan: manifest precedence must keep the row committed.
	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)
	row = findRow()
	if row == nil {
		t.Fatal("(3) manual row lost after rescan")
	}
	if row.Status != domain.StatusCommitted {
		t.Fatalf("(3) row status = %q after rescan, want %q (manager's own install probed as external)",
			row.Status, domain.StatusCommitted)
	}
	t.Log("(3) rescan kept the managed manual game committed")

	// 4. Uninstall of the manager's own install must succeed — no refusal.
	e.sess.Uninstall(custom)
	waitEvent(t, e.sess, EvOpDone)
	for _, toast := range e.sess.Snapshot().Toasts {
		if strings.Contains(toast.Text, notManagedRefusal) {
			t.Fatalf("(4) legitimate uninstall refused: %q", toast.Text)
		}
	}
	t.Log("(4) uninstall of the managed manual game succeeded")
}

// TestCanOpenINI: the OptiScaler.ini affordance opens for every install that
// has one on disk — manager-committed AND external (detected, unmanaged) —
// and stays closed for every state without a usable install.
func TestCanOpenINI(t *testing.T) {
	tests := []struct {
		name   string
		status domain.Status
		want   bool
	}{
		{"committed install", domain.StatusCommitted, true},
		{"external install", domain.StatusExternal, true},
		{"failed install", domain.StatusFailed, false},
		{"never installed", domain.Status(""), false},
		{"install in progress", domain.StatusInProgress, false},
		{"rolled back", domain.StatusRolledBack, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := GameRow{Status: tt.status}
			if got := row.CanOpenINI(); got != tt.want {
				t.Errorf("GameRow{Status: %q}.CanOpenINI() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
