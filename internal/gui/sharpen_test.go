package gui

// Issue 038: covers are never resized on disk — shirei scales them live at
// render from the decoded RGBA cached once per path in LoadImage. The 10%
// sharpen therefore lives at the decode seam: a vendor hook (v0.18) the GUI
// installs for cover-cache paths, applied once per load, never per frame.

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	. "go.hasen.dev/shirei"
)

// edgeRGBA builds a vertical step edge: left half gray l, right half gray r.
// A 3x3 box blur at the edge yields (2l+r)/3 on the left and (l+2r)/3 on the
// right, so a 10% unsharp mask moves the edge columns to exactly
// l-0.1(r-l)/3 and r+0.1(r-l)/3 (98 and 162 for l=100, r=160).
func edgeRGBA(w, h int, l, r uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := l
			if x >= w/2 {
				v = r
			}
			img.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	return img
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// decodeRGBA reads an image file back to RGBA the same way shirei's
// imageToRGBA does (draw.Draw is a no-op for an already-RGBA decode).
func decodeRGBA(t *testing.T, path string) *image.RGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba
	}
	b := src.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)
	return rgba
}

func TestSharpenFlatImageUnchanged(t *testing.T) {
	flat := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(flat, flat.Bounds(), image.NewUniform(color.RGBA{120, 130, 140, 200}), image.Point{}, draw.Src)

	got := sharpenRGBA(flat, 0.10)
	if !bytes.Equal(got.Pix, flat.Pix) {
		t.Error("sharpening a flat field must be a no-op (blur == pixel everywhere)")
	}
}

func TestSharpenEdgeBoostExact(t *testing.T) {
	src := edgeRGBA(64, 64, 100, 160)
	got := sharpenRGBA(src, 0.10)

	at := func(img *image.RGBA, x, y int) color.RGBA {
		i := img.PixOffset(x, y)
		return color.RGBA{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}
	}
	if p := at(got, 31, 32); p.R != 98 {
		t.Errorf("left edge column = %v, want R=98 (100 + 0.10·(100−120))", p)
	}
	if p := at(got, 32, 32); p.R != 162 {
		t.Errorf("right edge column = %v, want R=162 (160 + 0.10·(160−140))", p)
	}
	if p := at(got, 10, 32); p.R != 100 {
		t.Errorf("flat interior moved: %v, want R=100", p)
	}
	if p := at(got, 50, 32); p.R != 160 {
		t.Errorf("flat interior moved: %v, want R=160", p)
	}
}

func TestSharpenPreservesAlphaAndSource(t *testing.T) {
	src := edgeRGBA(64, 64, 100, 160)
	for y := 0; y < 64; y++ { // semi-transparent to prove alpha passes through
		for x := 0; x < 64; x++ {
			src.Pix[src.PixOffset(x, y)+3] = 128
		}
	}
	orig := make([]byte, len(src.Pix))
	copy(orig, src.Pix)

	got := sharpenRGBA(src, 0.10)
	if got == src || &got.Pix[0] == &src.Pix[0] {
		t.Error("sharpenRGBA must return a fresh buffer, never mutate the shared source")
	}
	if !bytes.Equal(src.Pix, orig) {
		t.Error("source pixels were mutated")
	}
	if a := got.Pix[got.PixOffset(31, 32)+3]; a != 128 {
		t.Errorf("alpha = %d, want 128 (unsharp mask touches RGB only)", a)
	}
}

// TestCoverDecodeHookSharpensCoversOnceOnLoad: a cover under the registered
// cache dir loads sharpened — exactly sharpenRGBA(raw, 0.10) — while an
// identical image outside the dir passes through untouched; a second
// LoadImage returns the same cached ImageData (no re-sharpen, no per-frame
// cost).
func TestCoverDecodeHookSharpensCoversOnceOnLoad(t *testing.T) {
	coversDir := t.TempDir()
	otherDir := t.TempDir()
	installCoverSharpen(coversDir)

	coverPath := filepath.Join(coversDir, "100.img")
	iconPath := filepath.Join(otherDir, "icon.png")
	writePNG(t, coverPath, edgeRGBA(64, 96, 100, 160))
	writePNG(t, iconPath, edgeRGBA(64, 96, 100, 160))

	data := LoadImage(coverPath)
	if data == nil || len(data.Pix) == 0 {
		t.Fatal("cover did not decode synchronously (small file)")
	}
	want := sharpenRGBA(decodeRGBA(t, coverPath), 0.10)
	if !bytes.Equal(data.Pix, want.Pix) {
		t.Error("cached cover pixels are not the 10% sharpen of the on-disk decode")
	}

	again := LoadImage(coverPath)
	if again != data {
		t.Error("second LoadImage re-decoded: sharpen must happen once, on the cached image")
	}

	icon := LoadImage(iconPath)
	raw := decodeRGBA(t, iconPath)
	if !bytes.Equal(icon.Pix, raw.Pix) {
		t.Error("non-cover image was sharpened — the hook must scope to the covers cache dir")
	}
}

// TestCoverDecodeHookAsyncLargeImage: files ≥500 KB decode on a background
// goroutine in LoadImage; the hook must sharpen that path too.
func TestCoverDecodeHookAsyncLargeImage(t *testing.T) {
	coversDir := t.TempDir()
	installCoverSharpen(coversDir)

	// Random noise compresses badly, so a modest canvas easily tops 500 KB.
	rng := rand.New(rand.NewSource(38))
	big := image.NewRGBA(image.Rect(0, 0, 800, 1200))
	for i := 0; i < len(big.Pix); i += 4 {
		big.Pix[i] = uint8(rng.Intn(256))
		big.Pix[i+1] = uint8(rng.Intn(256))
		big.Pix[i+2] = uint8(rng.Intn(256))
		big.Pix[i+3] = 255
	}
	coverPath := filepath.Join(coversDir, "big.img")
	writePNG(t, coverPath, big)
	if fi, _ := os.Stat(coverPath); fi.Size() < 500*1024 {
		t.Fatalf("noise PNG = %d bytes, want ≥500 KB to hit the async decode path", fi.Size())
	}

	data := LoadImage(coverPath)
	if data == nil {
		t.Fatal("LoadImage returned nil")
	}
	deadline := time.Now().Add(10 * time.Second)
	for len(data.Pix) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(data.Pix) == 0 {
		t.Fatal("async decode never landed")
	}
	want := sharpenRGBA(decodeRGBA(t, coverPath), 0.10)
	if !bytes.Equal(data.Pix, want.Pix) {
		t.Error("async-decoded cover pixels are not the 10% sharpen of the on-disk decode")
	}
}

// TestVendorImageDecodeHookPatch guards the v0.18 vendor patch: a `go mod
// vendor` refresh that silently drops it fails loudly here.
func TestVendorImageDecodeHookPatch(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	images := filepath.Join(filepath.Dir(file), "..", "..", "vendor", "go.hasen.dev", "shirei", "images.go")
	b, err := os.ReadFile(images)
	if err != nil {
		t.Fatalf("vendored images.go unreadable: %v", err)
	}
	if !strings.Contains(string(b), "PATCHED by optiscaler-manager (v0.18)") ||
		!strings.Contains(string(b), "func SetImageDecodeHook(") {
		t.Error("vendored images.go lacks the v0.18 image decode hook; reapply it (docs/vendor-patches.md)")
	}
}
