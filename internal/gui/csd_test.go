package gui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestVendorCSDPatchPresent: the vendored shirei carries the
// optiscaler-manager patch markers (CSD disabled, scroll speedup, Wayland
// Shift+Tab, Wayland client-side key repeat, Win32 client-side key repeat),
// so a `go mod vendor` refresh that silently drops them fails loudly here.
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
		t.Error("vendored shirei.go lacks the optiscaler-manager patches (scroll speedup, IdHasFocusWithin, resize animation snap); reapply them (docs/vendor-patches.md)")
	}
	if !strings.Contains(string(b), "windowResizedNow") {
		t.Error("vendored shirei.go lacks the v0.13 resize-animation-snap patch (windowResizedNow); reapply it (docs/vendor-patches.md)")
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager (v0.13)") {
		t.Error("vendored shirei.go lacks the v0.13 animation-disable patch; reapply it (docs/vendor-patches.md)")
	}

	softrender := filepath.Join(root, "vendor", "go.hasen.dev", "shirei", "softrender.go")
	b, err = os.ReadFile(softrender)
	if err != nil {
		t.Fatalf("vendored softrender.go unreadable: %v", err)
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager (v0.14)") {
		t.Error("vendored softrender.go lacks the v0.14 image stretch patch; reapply it (docs/vendor-patches.md)")
	}

	images := filepath.Join(root, "vendor", "go.hasen.dev", "shirei", "images.go")
	b, err = os.ReadFile(images)
	if err != nil {
		t.Fatalf("vendored images.go unreadable: %v", err)
	}
	if !strings.Contains(string(b), "func ImageFill(") {
		t.Error("vendored images.go lacks the v0.14 ImageFill function; reapply it (docs/vendor-patches.md)")
	}

	kbd := filepath.Join(root, "vendor", "go.hasen.dev", "shirei", "waylandbackend", "waylandkeyboard_linux.go")
	b, err = os.ReadFile(kbd)
	if err != nil {
		t.Fatalf("vendored waylandkeyboard_linux.go unreadable: %v", err)
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager") || !strings.Contains(string(b), "xkISOLeftTab") {
		t.Error("vendored waylandkeyboard_linux.go lacks the ISO_Left_Tab (Shift+Tab) patch; reapply it (docs/vendor-patches.md)")
	}
	if !strings.Contains(string(b), "func (*handler) HandleKeyboardRepeatInfo") || !strings.Contains(string(b), "pumpRepeat") {
		t.Error("vendored waylandkeyboard_linux.go lacks the v0.10 client-side key-repeat patch (HandleKeyboardRepeatInfo + pumpRepeat); reapply it (docs/vendor-patches.md)")
	}

	loop := filepath.Join(root, "vendor", "go.hasen.dev", "shirei", "waylandbackend", "waylandbackend_linux.go")
	b, err = os.ReadFile(loop)
	if err != nil {
		t.Fatalf("vendored waylandbackend_linux.go unreadable: %v", err)
	}
	if !strings.Contains(string(b), "pumpRepeat()") || !strings.Contains(string(b), "repeatTimeout(framePoll)") {
		t.Error("vendored waylandbackend_linux.go lacks the v0.10 key-repeat wiring (pumpRepeat / repeatTimeout); reapply it (docs/vendor-patches.md)")
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager (v0.12)") {
		t.Error("vendored waylandbackend_linux.go lacks the v0.12 resize-redraw patch (dirty=true in HandleToplevelConfigure); reapply it (docs/vendor-patches.md)")
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager (v0.15)") || !strings.Contains(string(b), "haveFrame") {
		t.Error("vendored waylandbackend_linux.go lacks the v0.15 skip-unchanged-frames patch (haveFrame); reapply it (docs/vendor-patches.md)")
	}

	win32 := filepath.Join(root, "vendor", "go.hasen.dev", "shirei", "win32backend", "win32backend_windows.go")
	b, err = os.ReadFile(win32)
	if err != nil {
		t.Fatalf("vendored win32backend_windows.go unreadable: %v", err)
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager (v0.11)") ||
		!strings.Contains(string(b), "func armRepeat(") ||
		!strings.Contains(string(b), "func pumpRepeat()") {
		t.Error("vendored win32backend_windows.go lacks the v0.11 client-side key-repeat patch (armRepeat / pumpRepeat); reapply it (docs/vendor-patches.md)")
	}
	t.Log("vendored patches present (CSD disabled, scroll speedup, Shift+Tab ISO_Left_Tab, Wayland key repeat, Win32 key repeat)")
}
