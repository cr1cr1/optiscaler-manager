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
// Patches v0.12 and v0.15 were superseded by shirei v0.6.10 upstream and are
// no longer reapplied (see docs/vendor-patches.md).
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
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager (v0.8)") {
		t.Error("vendored shirei.go lacks the v0.8 scroll-speedup patch; reapply it (docs/vendor-patches.md)")
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager (v0.13)") {
		t.Error("vendored shirei.go lacks the v0.13 animation-disable patch (rate=1 in resolveOrigins); reapply it (docs/vendor-patches.md)")
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
	renderpng := filepath.Join(root, "vendor", "go.hasen.dev", "shirei", "renderpng.go")
	b, err = os.ReadFile(renderpng)
	if err != nil {
		t.Fatalf("vendored renderpng.go unreadable: %v", err)
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager (v0.16)") {
		t.Error("vendored renderpng.go lacks the v0.16 identity-reset patch (test isolation); reapply it (docs/vendor-patches.md)")
	}
	// v0.17 (mouse cursor shape) spans four vendored files; every one must
	// carry the marker so a `go mod vendor` refresh fails loudly.
	for _, f := range []string{"shirei.go", "attrs.go", "waylandbackend/waylandcursor_linux.go", "waylandbackend/waylandbackend_linux.go", "waylandbackend/waylandinput_linux.go"} {
		pf, err := os.ReadFile(filepath.Join(root, "vendor", "go.hasen.dev", "shirei", f))
		if err != nil {
			t.Fatalf("read vendored %s: %v", f, err)
		}
		if !strings.Contains(string(pf), "PATCHED by optiscaler-manager (v0.17)") {
			t.Errorf("vendored %s lacks the v0.17 mouse-cursor-shape patch; reapply it (docs/vendor-patches.md)", f)
		}
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
	// v0.12 (resize-redraw) and v0.15 (skip-unchanged-frames) were local
	// patches that shirei v0.6.10 implements upstream (HandleSurfaceConfigure
	// sets dirty=true; drawFrame early-returns on an unchanged SurfacesHash).
	// The markers are gone on purpose; these checks now guard the upstream
	// mechanisms so an accidental downgrade fails loudly.
	if !strings.Contains(string(b), "ackSerial, hasAck = ev.Serial, true") || !strings.Contains(string(b), "dirty = true") {
		t.Error("vendored waylandbackend_linux.go lost the upstream resize-redraw (HandleSurfaceConfigure dirty=true); the v0.12 behavior regressed")
	}
	if !strings.Contains(string(b), "lastPresentedHash") {
		t.Error("vendored waylandbackend_linux.go lost the upstream skip-unchanged-frames (SurfacesHash early-return); the v0.15 behavior regressed")
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
