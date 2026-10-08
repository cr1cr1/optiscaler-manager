package covers

// Issue 039: the 10% cover sharpen moved out of the GUI's image-load path
// (issue 038 — registry sweeps re-applied it, and the sync decode path ran
// it on the UI thread) into the store seam: fetch/override sharpen the
// cached file once, a one-time New() migration sweeps legacy caches, and
// cache-hit reads never decode.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// edgePNG builds a vertical step edge (left half gray l, right half gray r)
// at an exact 2:3 aspect so normalizeCover passes it through untouched. A
// 3x3 box blur at the edge yields (2l+r)/3 left and (l+2r)/3 right, so a
// 10% unsharp mask moves the edge columns to exactly 98 and 162 for
// l=100, r=160.
func edgePNG(t *testing.T, w, h int, l, r uint8) []byte {
	t.Helper()
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
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pixelR(t *testing.T, path string, x, y int) uint8 {
	t.Helper()
	r, _, _, _ := decodeImg(t, path).At(x, y).RGBA()
	return uint8(r >> 8)
}

func TestFetchSharpensCoverAtStoreTime(t *testing.T) {
	f := newFakeCDN(t)
	f.coverBytes = edgePNG(t, 64, 96, 100, 160)
	c := NewWithBase(nil, t.TempDir(), f.srv.URL+"/steam/apps/%s/library_600x900.jpg", f.srv.URL+"/api/storesearch/")

	p, err := c.Cover(t.Context(), f.knownAppID, f.knownName)
	if err != nil {
		t.Fatal(err)
	}
	if got := pixelR(t, p, 31, 48); got != 98 {
		t.Errorf("left edge column R = %d, want 98 (100 + 0.10·(100−120)) — cached file must be the sharpened image", got)
	}
	if got := pixelR(t, p, 32, 48); got != 162 {
		t.Errorf("right edge column R = %d, want 162 (160 + 0.10·(160−140))", got)
	}
	if got := pixelR(t, p, 10, 48); got != 100 {
		t.Errorf("flat interior moved: R = %d, want 100", got)
	}
}

func TestFetchFlatCoverStaysFlat(t *testing.T) {
	f := newFakeCDN(t) // tinyPNG: flat 4x6 dark tile
	c := NewWithBase(nil, t.TempDir(), f.srv.URL+"/steam/apps/%s/library_600x900.jpg", f.srv.URL+"/api/storesearch/")

	p, err := c.Cover(t.Context(), f.knownAppID, f.knownName)
	if err != nil {
		t.Fatal(err)
	}
	if got := pixelR(t, p, 2, 3); got != 20 {
		t.Errorf("flat cover pixel R = %d, want 20 — sharpening a flat field is a no-op", got)
	}
}

// TestCoverCacheHitLeavesFileUntouched: the on-disk file is final after the
// store-time sharpen — a cache hit must not decode, re-encode, or re-sharpen.
func TestCoverCacheHitLeavesFileUntouched(t *testing.T) {
	f := newFakeCDN(t)
	f.coverBytes = edgePNG(t, 64, 96, 100, 160)
	c := NewWithBase(nil, t.TempDir(), f.srv.URL+"/steam/apps/%s/library_600x900.jpg", f.srv.URL+"/api/storesearch/")

	p, err := c.Cover(t.Context(), f.knownAppID, f.knownName)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Cover(t.Context(), f.knownAppID, f.knownName); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("cache-hit read changed the file bytes — sharpening must happen once, at store time")
	}
}

// TestNewMigratesLegacyCacheOnce: caches written before issue 039 hold
// unsharpened images; the first New() sweeps them exactly once (marker
// file), later constructions leave the bytes alone.
func TestNewMigratesLegacyCacheOnce(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "100.img")
	if err := os.WriteFile(legacy, edgePNG(t, 64, 96, 100, 160), 0o644); err != nil {
		t.Fatal(err)
	}

	New(nil, dir)
	if got := pixelR(t, legacy, 31, 48); got != 98 {
		t.Errorf("legacy left edge R = %d, want 98 after the one-time migration", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "_sharpened-v1")); err != nil {
		t.Errorf("migration marker %q missing: %v", "_sharpened-v1", err)
	}

	sharpened, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	New(nil, dir) // marker present: no second pass
	after, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sharpened, after) {
		t.Error("second New() re-sharpened — the migration must run exactly once")
	}
}

func TestSharpenRGBAFlatImageUnchanged(t *testing.T) {
	flat := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			flat.SetRGBA(x, y, color.RGBA{120, 130, 140, 200})
		}
	}
	got := sharpenRGBA(flat, 0.10)
	if !bytes.Equal(got.Pix, flat.Pix) {
		t.Error("sharpening a flat field must be a no-op (blur == pixel everywhere)")
	}
}

func TestSharpenRGBAEdgeBoostExact(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			v := uint8(100)
			if x >= 32 {
				v = 160
			}
			src.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
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

func TestSharpenRGBAPreservesAlphaAndSource(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := 3; i < len(src.Pix); i += 4 {
		src.Pix[i] = 128
	}
	orig := make([]byte, len(src.Pix))
	copy(orig, src.Pix)

	got := sharpenRGBA(src, 0.10)
	if got == src || &got.Pix[0] == &src.Pix[0] {
		t.Error("sharpenRGBA must return a fresh buffer, never mutate the source")
	}
	if !bytes.Equal(src.Pix, orig) {
		t.Error("source pixels were mutated")
	}
	if a := got.Pix[3]; a != 128 {
		t.Errorf("alpha = %d, want 128 (unsharp mask touches RGB only)", a)
	}
}
