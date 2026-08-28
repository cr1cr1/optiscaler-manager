package pever

import (
	"os"
	"path/filepath"
	"testing"
)

func rename(dir, from, to string) error {
	return os.Rename(filepath.Join(dir, from), filepath.Join(dir, to))
}

// The disable toggle renames a known hook file to <name>.disabled and back;
// detection must find the hook in either state and reject lookalikes.

func TestActiveHook(t *testing.T) {
	dir := t.TempDir()
	if got := ActiveHook(dir); got != "" {
		t.Fatalf("ActiveHook(empty dir) = %q, want \"\"", got)
	}
	writeCandidate(t, dir, "winmm.dll", identityPE(false, [2]string{"ProductName", "OptiScaler"}))
	if got := ActiveHook(dir); got != "winmm.dll" {
		t.Fatalf("ActiveHook = %q, want winmm.dll", got)
	}
	// A non-OptiScaler hook (e.g. DXVK's dxgi.dll) is not ours to rename.
	writeCandidate(t, dir, "dxgi.dll", identityPE(false, [2]string{"ProductName", "DXVK"}))
	if got := ActiveHook(dir); got != "winmm.dll" {
		t.Fatalf("ActiveHook with DXVK dxgi.dll = %q, want winmm.dll (identity gate)", got)
	}
	t.Log("active hook found by identity, DXVK lookalike skipped")
}

func TestDisabledHook(t *testing.T) {
	dir := t.TempDir()
	if got := DisabledHook(dir); got != "" {
		t.Fatalf("DisabledHook(empty dir) = %q, want \"\"", got)
	}
	// An active hook alone is not disabled.
	writeCandidate(t, dir, "dxgi.dll", identityPE(false, [2]string{"ProductName", "OptiScaler"}))
	if got := DisabledHook(dir); got != "" {
		t.Fatalf("DisabledHook(active only) = %q, want \"\"", got)
	}
	// Renaming it away flips the answer to the disabled name.
	if err := rename(dir, "dxgi.dll", "dxgi.dll"+DisabledSuffix); err != nil {
		t.Fatal(err)
	}
	if got := DisabledHook(dir); got != "dxgi.dll" {
		t.Fatalf("DisabledHook = %q, want dxgi.dll", got)
	}
	// And ActiveHook no longer sees it.
	if got := ActiveHook(dir); got != "" {
		t.Fatalf("ActiveHook(disabled) = %q, want \"\"", got)
	}
	t.Log("disabled hook detected via .disabled suffix, active probe blind to it")
}

func TestHookCandidatesCoverASI(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "OptiScaler.asi", identityPE(false, [2]string{"OriginalFilename", "OptiScaler.asi"}))
	if got := ActiveHook(dir); got != "OptiScaler.asi" {
		t.Fatalf("ActiveHook(.asi) = %q, want OptiScaler.asi", got)
	}
	t.Log("OptiScaler.asi is a known hook")
}
