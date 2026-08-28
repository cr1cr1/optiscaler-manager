package pever

import (
	"os"
	"path/filepath"
	"testing"
)

func rename(dir, from, to string) error {
	return os.Rename(filepath.Join(dir, from), filepath.Join(dir, to))
}

func writeFile(dir, name string, data []byte) error {
	return os.WriteFile(filepath.Join(dir, name), data, 0o644)
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

// The spec is unconditional: a known hook name carrying the .disabled
// suffix means disabled, even when the file cannot be identity-verified
// (hand-renamed, stripped, not a PE). Presence is enough.
func TestDisabledHookPresenceIsEnough(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(dir, "winmm.dll"+DisabledSuffix, []byte("not a PE")); err != nil {
		t.Fatal(err)
	}
	if got := DisabledHook(dir); got != "winmm.dll" {
		t.Fatalf("DisabledHook(unverifiable file) = %q, want winmm.dll (presence is enough)", got)
	}
	t.Log("presence-only detection honors the spec's obvious clause")
}

// First-time external detection of an unmanaged dir is the exception: a
// DXVK dxgi.dll.disabled must not label the game as an OptiScaler install,
// so the discovery path verifies identity.
func TestDisabledHookVerified(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(dir, "dxgi.dll"+DisabledSuffix, []byte("not a PE")); err != nil {
		t.Fatal(err)
	}
	if got := DisabledHookVerified(dir); got != "" {
		t.Fatalf("DisabledHookVerified(non-PE) = %q, want \"\" (no identity)", got)
	}
	writeCandidate(t, dir, "winmm.dll"+DisabledSuffix, identityPE(false, [2]string{"ProductName", "DXVK"}))
	if got := DisabledHookVerified(dir); got != "" {
		t.Fatalf("DisabledHookVerified(DXVK) = %q, want \"\" (not OptiScaler)", got)
	}
	writeCandidate(t, dir, "version.dll"+DisabledSuffix, identityPE(false, [2]string{"ProductName", "OptiScaler"}))
	if got := DisabledHookVerified(dir); got != "version.dll" {
		t.Fatalf("DisabledHookVerified(branded) = %q, want version.dll", got)
	}
	t.Log("verified variant keeps the DXVK gate for discovery")
}

func TestHookCandidatesCoverASI(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "OptiScaler.asi", identityPE(false, [2]string{"OriginalFilename", "OptiScaler.asi"}))
	if got := ActiveHook(dir); got != "OptiScaler.asi" {
		t.Fatalf("ActiveHook(.asi) = %q, want OptiScaler.asi", got)
	}
	t.Log("OptiScaler.asi is a known hook")
}
