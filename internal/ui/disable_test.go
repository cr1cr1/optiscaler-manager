package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// mkHookGame builds a session row for an installed game whose injection dir
// holds an OptiScaler-branded dxgi.dll, and returns both.
func mkHookGame(t *testing.T, s *Session, title string) string {
	t.Helper()
	dir := t.TempDir()
	hook := filepath.Join(dir, "dxgi.dll")
	if err := os.WriteFile(hook, testutil.StringInfoPE(false, map[string]string{"ProductName": "OptiScaler"}, [4]uint16{}), 0o644); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.st.Rows = append(s.st.Rows, GameRow{
		Title:        title,
		InstallDir:   dir,
		InjectionDir: dir,
		Status:       domain.StatusCommitted,
	})
	s.mu.Unlock()
	return dir
}

func TestToggleDisabledRoundTrip(t *testing.T) {
	s := NewSession(Deps{})
	dir := mkHookGame(t, s, "Hook Game")

	s.ToggleDisabled(dir)
	if _, err := os.Stat(filepath.Join(dir, "dxgi.dll"+pever.DisabledSuffix)); err != nil {
		t.Fatalf("hook not renamed away: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dxgi.dll")); !os.IsNotExist(err) {
		t.Fatalf("active hook still present after disable: %v", err)
	}
	if row := s.findRow(dir); row == nil || !row.Disabled {
		t.Fatalf("row.Disabled = %+v after disable, want true", row)
	}

	s.ToggleDisabled(dir)
	if _, err := os.Stat(filepath.Join(dir, "dxgi.dll")); err != nil {
		t.Fatalf("hook not restored after enable: %v", err)
	}
	if row := s.findRow(dir); row == nil || row.Disabled {
		t.Fatalf("row.Disabled = %+v after enable, want false", row)
	}

	var texts []string
	for _, to := range s.Snapshot().Toasts {
		texts = append(texts, to.Text)
	}
	t.Logf("toasts: %v", texts)
}

func TestToggleDisabledNoHook(t *testing.T) {
	s := NewSession(Deps{})
	dir := t.TempDir()
	s.mu.Lock()
	s.st.Rows = append(s.st.Rows, GameRow{
		Title:        "Plain Game",
		InstallDir:   dir,
		InjectionDir: dir,
		Status:       domain.StatusCommitted,
	})
	s.mu.Unlock()

	s.ToggleDisabled(dir)
	snap := s.Snapshot()
	if len(snap.Toasts) == 0 || !snap.Toasts[len(snap.Toasts)-1].Warn {
		t.Fatalf("no warn toast for hook-less toggle: %+v", snap.Toasts)
	}
	if !strings.Contains(snap.Toasts[len(snap.Toasts)-1].Text, "no OptiScaler hook") {
		t.Fatalf("toast %q, want a no-hook explanation", snap.Toasts[len(snap.Toasts)-1].Text)
	}
	if row := s.findRow(dir); row == nil || row.Disabled {
		t.Fatalf("row.Disabled = %+v, want false (nothing renamed)", row)
	}
	t.Logf("hook-less toggle refused: %q", snap.Toasts[len(snap.Toasts)-1].Text)
}

// TestStartReconcilesDisabledFlag: a warm boot re-probes the disabled state
// from disk, so a toggle done outside the running session (or by an older
// cache) still renders as disabled.
func TestStartReconcilesDisabledFlag(t *testing.T) {
	e := newTestEnv(t)
	root := t.TempDir()
	e.sess.deps.SettingsRoot = root
	gameDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(gameDir, "dxgi.dll"+pever.DisabledSuffix),
		testutil.StringInfoPE(false, map[string]string{"ProductName": "OptiScaler"}, [4]uint16{}), 0o644); err != nil {
		t.Fatal(err)
	}
	writeGamesCache(t, root, []GameRow{{
		Title:        "Disabled Game",
		AppID:        "custom_DisabledGame",
		InstallDir:   gameDir,
		InjectionDir: gameDir,
		Status:       domain.StatusCommitted,
	}})
	if err := e.sess.deps.Store.Save(&domain.Manifest{
		ID:         domain.ManifestID(gameDir),
		Status:     domain.StatusCommitted,
		GameRoot:   gameDir,
		InstallDir: gameDir,
	}); err != nil {
		t.Fatal(err)
	}

	e.sess.Start(context.Background())

	row := e.sess.findRow(gameDir)
	if row == nil || !row.Disabled {
		t.Fatalf("warm-boot row.Disabled = %+v, want true", row)
	}
	t.Log("warm boot reconciled the disabled flag from disk")
}

// TestScanMapsDisabledToRow: the scan pipeline carries LibraryEntry.Disabled
// onto the GameRow frontends render.
func TestScanMapsDisabledToRow(t *testing.T) {
	e := newTestEnv(t)
	if err := os.WriteFile(filepath.Join(e.bin, "dxgi.dll"+pever.DisabledSuffix),
		testutil.StringInfoPE(false, map[string]string{"ProductName": "OptiScaler"}, [4]uint16{}), 0o644); err != nil {
		t.Fatal(err)
	}

	e.sess.Scan(context.Background())
	waitEvent(t, e.sess, EvScanDone)

	row := e.sess.findRow(e.gameRoot)
	if row == nil {
		t.Fatal("scanned row missing")
	}
	if row.Status != domain.StatusExternal || !row.Disabled {
		t.Fatalf("row = status %q disabled %v, want external+disabled", row.Status, row.Disabled)
	}
	t.Logf("scan mapped disabled hook: status %q", row.Status)
}

