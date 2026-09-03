package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// addPlainRow seeds a row with no OptiScaler install on record.
func addPlainRow(s *Session, title, installDir, injectionDir string) {
	s.mu.Lock()
	s.st.Rows = append(s.st.Rows, GameRow{
		Title:        title,
		InstallDir:   installDir,
		InjectionDir: injectionDir,
	})
	s.mu.Unlock()
}

// TestSelectReprobesHandToggledHook: the user renamed the hook to
// .disabled by hand while the session held cached state; selecting the
// game re-probes the disk and the row (and its toggle caption) flip
// without a rescan. Selecting again after a hand-enable flips back.
func TestSelectReprobesHandToggledHook(t *testing.T) {
	s := NewSession(Deps{})
	dir := mkHookGame(t, s, "Hook Game") // committed, active dxgi.dll

	if err := os.Rename(filepath.Join(dir, "dxgi.dll"), filepath.Join(dir, "dxgi.dll"+pever.DisabledSuffix)); err != nil {
		t.Fatal(err)
	}
	s.Select(dir)
	row := s.findRow(dir)
	if row == nil || !row.Disabled {
		t.Fatalf("row.Disabled = %+v after hand-disable, want true", row)
	}
	if label, ok := row.DisableToggleLabel(); !ok || label != "Enable OptiScaler" {
		t.Fatalf("toggle label = %q ok=%v, want %q", label, ok, "Enable OptiScaler")
	}

	if err := os.Rename(filepath.Join(dir, "dxgi.dll"+pever.DisabledSuffix), filepath.Join(dir, "dxgi.dll")); err != nil {
		t.Fatal(err)
	}
	s.Select(dir)
	row = s.findRow(dir)
	if row == nil || row.Disabled {
		t.Fatalf("row.Disabled = %+v after hand-enable, want false", row)
	}
	t.Log("hand-toggled hook reflected on selection")
}

// TestSelectDetectsManualInstall: OptiScaler was dropped into the game
// directory by hand after the scan; selecting the game surfaces the
// external install (status + toggle affordance) without a rescan.
func TestSelectDetectsManualInstall(t *testing.T) {
	s := NewSession(Deps{})
	dir := t.TempDir()
	addPlainRow(s, "Plain Game", dir, dir)

	hook := filepath.Join(dir, "dxgi.dll")
	if err := os.WriteFile(hook, testutil.StringInfoPE(false, map[string]string{"ProductName": "OptiScaler"}, [4]uint16{0, 9, 4, 0}), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Select(dir)

	row := s.findRow(dir)
	if row == nil || row.Status != domain.StatusExternal || row.Disabled {
		t.Fatalf("row = %+v, want external not-disabled", row)
	}
	if !row.HasInstall() {
		t.Fatal("row.HasInstall = false, want true after detection")
	}
	t.Logf("manual install detected on selection: status %q version %q", row.Status, row.OptiScalerVersion)
}

// TestSelectDetectsManualDisabledInstall: the hand-dropped hook is already
// renamed away; the row must surface as external AND disabled so the
// toggle offers Enable, not Disable.
func TestSelectDetectsManualDisabledInstall(t *testing.T) {
	s := NewSession(Deps{})
	dir := t.TempDir()
	addPlainRow(s, "Plain Game", dir, dir)

	hook := filepath.Join(dir, "winmm.dll"+pever.DisabledSuffix)
	if err := os.WriteFile(hook, testutil.StringInfoPE(false, map[string]string{"ProductName": "OptiScaler"}, [4]uint16{}), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Select(dir)

	row := s.findRow(dir)
	if row == nil || row.Status != domain.StatusExternal || !row.Disabled {
		t.Fatalf("row = %+v, want external+disabled", row)
	}
	if label, ok := row.DisableToggleLabel(); !ok || label != "Enable OptiScaler" {
		t.Fatalf("toggle label = %q ok=%v, want %q", label, ok, "Enable OptiScaler")
	}
	t.Log("hand-disabled manual install detected on selection")
}

// TestSelectClearsHandRemovedExternalInstall: an external install whose
// hook the user deleted by hand must stop rendering as installed.
func TestSelectClearsHandRemovedExternalInstall(t *testing.T) {
	s := NewSession(Deps{})
	dir := t.TempDir()
	hook := filepath.Join(dir, "dxgi.dll")
	if err := os.WriteFile(hook, testutil.StringInfoPE(false, map[string]string{"ProductName": "OptiScaler"}, [4]uint16{}), 0o644); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.st.Rows = append(s.st.Rows, GameRow{
		Title:        "External Game",
		InstallDir:   dir,
		InjectionDir: dir,
		Status:       domain.StatusExternal,
	})
	s.mu.Unlock()

	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	s.Select(dir)

	row := s.findRow(dir)
	if row == nil || row.Status != "" || row.Disabled || row.HasInstall() {
		t.Fatalf("row = %+v, want no install after hand removal", row)
	}
	t.Log("hand-removed external install cleared on selection")
}

// TestSelectResolvesInjectionDir: a cached row with no injection dir
// (resolution failed at scan time, e.g. the exe arrived later) still
// detects a hand-dropped hook by resolving the install dir on selection.
func TestSelectResolvesInjectionDir(t *testing.T) {
	s := NewSession(Deps{})
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "game.exe"), []byte("MZGAME"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "dxgi.dll"), testutil.StringInfoPE(false, map[string]string{"ProductName": "OptiScaler"}, [4]uint16{}), 0o644); err != nil {
		t.Fatal(err)
	}
	addPlainRow(s, "Late Exe Game", dir, "")

	s.Select(dir)

	row := s.findRow(dir)
	if row == nil || row.Status != domain.StatusExternal {
		t.Fatalf("row = %+v, want external", row)
	}
	if row.InjectionDir != bin {
		t.Fatalf("row.InjectionDir = %q, want %q", row.InjectionDir, bin)
	}
	t.Log("selection resolved the injection dir and detected the hook")
}

// TestSelectFollowsHandRenamedHook: the user renamed the entrypoint DLL
// to a different known hook name (dxgi.dll → winmm.dll) by hand. The
// probes scan the whole candidate set on every call, so the row keeps
// its install state and the disable toggle renames the NEW name.
func TestSelectFollowsHandRenamedHook(t *testing.T) {
	s := NewSession(Deps{})
	dir := mkHookGame(t, s, "Hook Game") // committed, active dxgi.dll

	if err := os.Rename(filepath.Join(dir, "dxgi.dll"), filepath.Join(dir, "winmm.dll")); err != nil {
		t.Fatal(err)
	}
	s.Select(dir)
	row := s.findRow(dir)
	if row == nil || row.Status != domain.StatusCommitted || row.Disabled {
		t.Fatalf("row = %+v after rename, want committed not-disabled", row)
	}

	// The toggle must rename the new entrypoint, not the remembered one.
	s.ToggleDisabled(dir)
	if _, err := os.Stat(filepath.Join(dir, "winmm.dll"+pever.DisabledSuffix)); err != nil {
		t.Fatalf("renamed hook not disabled: %v", err)
	}
	if row := s.findRow(dir); row == nil || !row.Disabled {
		t.Fatalf("row.Disabled = %+v after toggle, want true", row)
	}
	t.Log("selection and toggle followed the hand-renamed hook")
}

// TestSelectFollowsHandRenamedDisabledHook: same rename, but the user
// also kept it disabled (dxgi.dll.disabled → winmm.dll.disabled).
func TestSelectFollowsHandRenamedDisabledHook(t *testing.T) {
	s := NewSession(Deps{})
	dir := mkHookGame(t, s, "Hook Game")

	if err := os.Rename(filepath.Join(dir, "dxgi.dll"), filepath.Join(dir, "winmm.dll"+pever.DisabledSuffix)); err != nil {
		t.Fatal(err)
	}
	s.Select(dir)
	row := s.findRow(dir)
	if row == nil || row.Status != domain.StatusCommitted || !row.Disabled {
		t.Fatalf("row = %+v after rename, want committed+disabled", row)
	}

	// Enable must restore the new name, not the old one.
	s.ToggleDisabled(dir)
	if _, err := os.Stat(filepath.Join(dir, "winmm.dll")); err != nil {
		t.Fatalf("renamed hook not re-enabled: %v", err)
	}
	t.Log("selection and toggle followed the hand-renamed disabled hook")
}
