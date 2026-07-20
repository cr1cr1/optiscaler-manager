package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// waitScanDone drains events until the scan settles; an empty library must
// arrive as EvScanDone (guidance path), never EvScanFailed (error path).
func waitScanDone(t *testing.T, s *Session) Event {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case ev := <-s.Events():
			t.Logf("event: %v %q", ev.Kind, ev.Text)
			if ev.Kind == EvScanFailed {
				t.Fatalf("scan reported as failure: %q", ev.Text)
			}
			if ev.Kind == EvScanDone {
				return ev
			}
		case <-deadline:
			t.Fatal("timed out waiting for scan to settle")
		}
	}
}

// TestFirstRunZeroConfigScanToInstalled is the grandma-proof first-run E2E:
// a completely fresh profile (empty data/cache/settings roots) auto-scan
// finds the fixture game, and ONE QuickInstall click ends with a committed
// status and a real bundle installed into the game directory.
func TestFirstRunZeroConfigScanToInstalled(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir() // fresh profile: no settings file
	t.Logf("first-run profile: cache=%s settings=%s (both empty)",
		e.sess.deps.CacheDir, e.sess.deps.SettingsRoot)

	// 1. Zero-config scan (no settings modal, no version pick).
	e.sess.Start(context.Background())
	scanEv := waitScanDone(t, e.sess)
	st := e.sess.Snapshot()
	if len(st.Rows) != 1 {
		t.Fatalf("scan rows = %d, want 1 fixture game", len(st.Rows))
	}
	row := st.Rows[0]
	t.Logf("scan result: %q → row %q at %s (status %q, EAC=%v)",
		scanEv.Text, row.Title, row.InstallDir, row.Status, row.EAC)

	// 2. ONE click: QuickInstall on the not-installed row.
	e.sess.QuickInstall(row.InstallDir)
	opEv := waitEvent(t, e.sess, EvOpDone)
	t.Logf("op result: %q", opEv.Text)

	// 3. Status committed AND the real bundle landed in the game bin dir.
	st = e.sess.Snapshot()
	var final *GameRow
	for i := range st.Rows {
		if st.Rows[i].InstallDir == row.InstallDir {
			final = &st.Rows[i]
		}
	}
	if final == nil || final.Status != domain.StatusCommitted {
		t.Fatalf("final row status = %+v, want committed", final)
	}
	dxgi := filepath.Join(e.bin, "dxgi.dll")
	if _, err := os.Stat(dxgi); err != nil {
		t.Fatalf("dxgi.dll not installed into game bin dir: %v", err)
	}
	t.Logf("E2E transcript: scan(%q) → 1 click QuickInstall → status=%q, %s present",
		scanEv.Text, final.Status, dxgi)
}

// TestFirstRunEACInlinePrompt proves the EAC safety gate is inline and one
// extra click: QuickInstall on an EAC game shows the confirm prompt WITHOUT
// installing, and accepting it proceeds to a real install.
func TestFirstRunEACInlinePrompt(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	writeUIFile(t, filepath.Join(e.gameRoot, "start_protected_game.exe"), "EAC")

	e.sess.Start(context.Background())
	waitScanDone(t, e.sess)
	row := e.sess.Snapshot().Rows[0]
	if !row.EAC {
		t.Fatalf("fixture row not flagged EAC: %+v", row)
	}
	t.Logf("scan result: EAC game %q detected", row.Title)

	// QuickInstall must NOT silently install: it parks on an inline confirm.
	e.sess.QuickInstall(row.InstallDir)
	confirmEv := waitEvent(t, e.sess, EvConfirm)
	t.Logf("inline prompt: %q", confirmEv.Text)

	st := e.sess.Snapshot()
	if st.Confirm == nil || st.Confirm.Kind != ConfirmEAC {
		t.Fatalf("expected pending inline EAC confirmation, got %+v", st.Confirm)
	}
	dxgi := filepath.Join(e.bin, "dxgi.dll")
	if _, err := os.Stat(dxgi); !os.IsNotExist(err) {
		t.Fatal("install proceeded before the EAC prompt was answered")
	}
	t.Log("install NOT started while prompt pending (no dxgi.dll yet)")

	// One extra click: accept → real install completes.
	e.sess.AnswerConfirm(true)
	opEv := waitEvent(t, e.sess, EvOpDone)
	t.Logf("op result after consent: %q", opEv.Text)
	if _, err := os.Stat(dxgi); err != nil {
		t.Fatalf("dxgi.dll not installed after EAC consent: %v", err)
	}
	st = e.sess.Snapshot()
	if st.Confirm != nil {
		t.Fatal("confirm prompt not cleared after consent")
	}
	if st.Rows[0].Status != domain.StatusCommitted {
		t.Fatalf("final status %q, want committed", st.Rows[0].Status)
	}
	t.Logf("E2E transcript: scan(EAC) → QuickInstall → inline confirm → accept → committed, %s present", dxgi)
}

// TestFirstRunEmptyLibraryScanSucceeds: a first-run user with zero games
// must get a settled empty library (EvScanDone, 0 rows) — never a scary
// "Scan failed" error — so the frontend can show empty-state guidance.
func TestFirstRunEmptyLibraryScanSucceeds(t *testing.T) {
	e := newTestEnv(t)
	e.sess.deps.SettingsRoot = t.TempDir()
	// Remove every game: valid Steam root, empty library.
	if err := os.Remove(filepath.Join(e.steamRoot, "steamapps", "appmanifest_100.acf")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(e.steamRoot, "steamapps", "common")); err != nil {
		t.Fatal(err)
	}

	e.sess.Start(context.Background())
	ev := waitScanDone(t, e.sess)
	st := e.sess.Snapshot()
	if len(st.Rows) != 0 {
		t.Fatalf("empty library rows = %d, want 0", len(st.Rows))
	}
	if !strings.Contains(ev.Text, "0 games") {
		t.Errorf("scan-done text %q, want \"0 games\"", ev.Text)
	}
	for _, toast := range st.Toasts {
		if toast.Warn {
			t.Errorf("warning toast on a clean empty first run: %q", toast.Text)
		}
	}
	t.Logf("empty first run: scan done %q, 0 rows, no warning toasts", ev.Text)
}
