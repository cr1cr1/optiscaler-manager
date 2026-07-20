package gui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestVendorCSDPatchPresent: the vendored shirei Wayland CSD titlebar
// carries the optiscaler-manager dark-theme patch marker, so a `go mod
// vendor` refresh that silently drops the patch fails loudly here.
func TestVendorCSDPatchPresent(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	p := filepath.Join(root, "vendor", "go.hasen.dev", "shirei", "waylandbackend", "waylanddecor_linux.go")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("vendored waylanddecor_linux.go unreadable: %v", err)
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager") {
		t.Error("vendored waylanddecor_linux.go lacks the dark-CSD patch marker; reapply the patch (docs/vendor-patches.md)")
	}
	t.Log("vendored CSD patch marker present")
}
