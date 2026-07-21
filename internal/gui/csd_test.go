package gui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestVendorPatchesPresent: the vendored shirei carries the
// optiscaler-manager patch markers (CSD disabled, scroll speedup), so a
// `go mod vendor` refresh that silently drops them fails loudly here.
func TestVendorCSDPatchPresent(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")

	decor := filepath.Join(root, "vendor", "go.hasen.dev", "shirei", "waylandbackend", "waylanddecor_linux.go")
	b, err := os.ReadFile(decor)
	if err != nil {
		t.Fatalf("vendored waylanddecor_linux.go unreadable: %v", err)
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager") || !strings.Contains(string(b), "csdEnabled = false") {
		t.Error("vendored waylanddecor_linux.go lacks the CSD-disable patch; reapply it (docs/vendor-patches.md)")
	}

	core := filepath.Join(root, "vendor", "go.hasen.dev", "shirei", "shirei.go")
	b, err = os.ReadFile(core)
	if err != nil {
		t.Fatalf("vendored shirei.go unreadable: %v", err)
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager") {
		t.Error("vendored shirei.go lacks the scroll-speedup patch; reapply it (docs/vendor-patches.md)")
	}
	t.Log("vendored patches present (CSD disabled, scroll speedup)")
}
