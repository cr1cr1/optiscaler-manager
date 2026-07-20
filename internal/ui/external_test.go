package ui

import (
	"context"
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
