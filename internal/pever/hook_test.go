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
	if hook, file := DisabledHook(dir); file != "" {
		t.Fatalf("DisabledHook(empty dir) = %q/%q, want \"\"", hook, file)
	}
	// An active hook alone is not disabled.
	writeCandidate(t, dir, "dxgi.dll", identityPE(false, [2]string{"ProductName", "OptiScaler"}))
	if hook, file := DisabledHook(dir); file != "" {
		t.Fatalf("DisabledHook(active only) = %q/%q, want \"\"", hook, file)
	}
	// Renaming it away flips the answer to the disabled name.
	if err := rename(dir, "dxgi.dll", "dxgi.dll"+DisabledSuffix); err != nil {
		t.Fatal(err)
	}
	if hook, file := DisabledHook(dir); hook != "dxgi.dll" || file != "dxgi.dll"+DisabledSuffix {
		t.Fatalf("DisabledHook = %q/%q, want dxgi.dll/dxgi.dll.disabled", hook, file)
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
	if hook, file := DisabledHook(dir); hook != "winmm.dll" || file != "winmm.dll"+DisabledSuffix {
		t.Fatalf("DisabledHook(unverifiable file) = %q/%q, want winmm.dll/winmm.dll.disabled (presence is enough)", hook, file)
	}
	t.Log("presence-only detection honors the spec's obvious clause")
}

// A hand-renamed hook carries ANY suffix the user picked (.1, .bak, .old):
// a known hook name plus a suffix is a renamed-away hook regardless of the
// suffix's spelling, so backup-style renames also read as disabled.
func TestDisabledHookArbitrarySuffix(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(dir, "dxgi.dll.1", []byte("not a PE")); err != nil {
		t.Fatal(err)
	}
	if hook, file := DisabledHook(dir); hook != "dxgi.dll" || file != "dxgi.dll.1" {
		t.Fatalf("DisabledHook(.1) = %q/%q, want dxgi.dll/dxgi.dll.1", hook, file)
	}
	// A suffixed name that is no known hook is not a hook at all.
	if err := rename(dir, "dxgi.dll.1", "random.dll.1"); err != nil {
		t.Fatal(err)
	}
	if hook, file := DisabledHook(dir); file != "" {
		t.Fatalf("DisabledHook(random.dll.1) = %q/%q, want \"\"", hook, file)
	}
	t.Log("arbitrary suffix on a known hook name reads as disabled")
}

// The manager's own .disabled suffix wins over backup-style renames so the
// toggle round-trips deterministically when both are present.
func TestDisabledHookPrefersManagerSuffix(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(dir, "dxgi.dll.1", []byte("not a PE")); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(dir, "winmm.dll"+DisabledSuffix, []byte("not a PE")); err != nil {
		t.Fatal(err)
	}
	if hook, file := DisabledHook(dir); hook != "winmm.dll" || file != "winmm.dll"+DisabledSuffix {
		t.Fatalf("DisabledHook = %q/%q, want the manager-suffixed winmm.dll.disabled", hook, file)
	}
	t.Log("exact .disabled suffix preferred over arbitrary suffixes")
}

// First-time external detection of an unmanaged dir is the exception: a
// DXVK dxgi.dll.disabled must not label the game as an OptiScaler install,
// so the discovery path verifies identity.
func TestDisabledHookVerified(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(dir, "dxgi.dll"+DisabledSuffix, []byte("not a PE")); err != nil {
		t.Fatal(err)
	}
	if hook, file := DisabledHookVerified(dir); file != "" {
		t.Fatalf("DisabledHookVerified(non-PE) = %q/%q, want \"\" (no identity)", hook, file)
	}
	writeCandidate(t, dir, "winmm.dll"+DisabledSuffix, identityPE(false, [2]string{"ProductName", "DXVK"}))
	if hook, file := DisabledHookVerified(dir); file != "" {
		t.Fatalf("DisabledHookVerified(DXVK) = %q/%q, want \"\" (not OptiScaler)", hook, file)
	}
	writeCandidate(t, dir, "version.dll"+DisabledSuffix, identityPE(false, [2]string{"ProductName", "OptiScaler"}))
	if hook, file := DisabledHookVerified(dir); hook != "version.dll" || file != "version.dll"+DisabledSuffix {
		t.Fatalf("DisabledHookVerified(branded) = %q/%q, want version.dll/version.dll.disabled", hook, file)
	}
	t.Log("verified variant keeps the DXVK gate for discovery")
}

// The identity gate extends to arbitrary suffixes: a branded dxgi.dll.bak
// is a disabled OptiScaler install, a DXVK dxgi.dll.1 is not.
func TestDisabledHookVerifiedArbitrarySuffix(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "dxgi.dll.bak", identityPE(false, [2]string{"ProductName", "OptiScaler"}))
	if hook, file := DisabledHookVerified(dir); hook != "dxgi.dll" || file != "dxgi.dll.bak" {
		t.Fatalf("DisabledHookVerified(.bak branded) = %q/%q, want dxgi.dll/dxgi.dll.bak", hook, file)
	}
	writeCandidate(t, dir, "winmm.dll.1", identityPE(false, [2]string{"ProductName", "DXVK"}))
	if hook, _ := DisabledHookVerified(dir); hook == "winmm.dll" {
		t.Fatalf("DisabledHookVerified(DXVK .1) matched %q, want the DXVK backup skipped", hook)
	}
	t.Log("verified arbitrary suffix keeps the DXVK gate")
}

func TestHookCandidatesCoverASI(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "OptiScaler.asi", identityPE(false, [2]string{"OriginalFilename", "OptiScaler.asi"}))
	if got := ActiveHook(dir); got != "OptiScaler.asi" {
		t.Fatalf("ActiveHook(.asi) = %q, want OptiScaler.asi", got)
	}
	t.Log("OptiScaler.asi is a known hook")
}
