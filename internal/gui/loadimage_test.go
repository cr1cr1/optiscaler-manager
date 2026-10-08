package gui

import (
	. "go.hasen.dev/shirei"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// A missing image file must not be pinned by the registry: when the file
// later appears at the SAME path (a cover re-fetched after a cache wipe,
// issue 046), the next LoadImage must decode it. Regression: LoadImage
// registered and file-cached a pixel-less ImageData for the missing file;
// the visible card touched that entry every frame so the sweep never
// reclaimed it — posters stayed on the generic filler even though the
// rescan had re-downloaded the file.
func TestLoadImageRecoversWhenFileAppears(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cover.img")

	first := LoadImage(p)
	if first == nil {
		t.Fatal("LoadImage(missing) returned nil")
	}
	if len(first.RGBA.Pix) != 0 {
		t.Fatalf("missing file decoded to %d pixels, want none", len(first.RGBA.Pix))
	}

	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 4, 6))
	for i := range img.Pix {
		img.Pix[i] = uint8(20 + i%200)
	}
	img.Set(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 255})
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	second := LoadImage(p)
	if second == nil {
		t.Fatal("LoadImage(existing) returned nil")
	}
	if len(second.RGBA.Pix) == 0 {
		t.Fatal("pixel-less image returned after the file appeared at the same path")
	}
	if second.Generation == 0 {
		t.Error("Generation still 0 after the file appeared — the pixels-changed signal never fired")
	}
	t.Logf("recovered: %dx%d, generation %d", second.RGBA.Rect.Dx(), second.RGBA.Rect.Dy(), second.Generation)
}
