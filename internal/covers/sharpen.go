package covers

// Issue 039: cover sharpening happens once, at store time, on the cached
// file — never at render or image-load time (issue 038's decode hook
// re-applied it on every registry reload and ran on the UI thread; it was
// ripped out). fetch and SetOverride call sharpenCover right after
// normalizeCover; New runs a one-time marker-gated migration so caches
// written before this change are sharpened exactly once. Cache-hit reads
// never decode: the bytes on disk are final.

import (
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// sharpenAmount is the unsharp-mask strength: out = p + amount·(p−blur).
const sharpenAmount = 0.10

// sharpenedMarker is the cacheDir file whose presence means every cached
// cover is already sharpened (store-time for new writes, migrated for
// legacy ones).
const sharpenedMarker = "_sharpened-v1"

// sharpenLegacyCache sharpens every cached cover written before the
// store-time sharpen existed, exactly once. Synchronous at construction:
// New runs before any scan goroutine, so the sweep never races a fetch —
// which matters because a double-sharpened poster is worse than a soft
// one. The marker is written after the sweep; a crash mid-sweep re-runs it
// (some files sharpen twice — cosmetic, and the window is seconds).
func sharpenLegacyCache(cacheDir string) {
	if _, err := os.Stat(filepath.Join(cacheDir, sharpenedMarker)); err == nil {
		return
	}
	entries, err := os.ReadDir(cacheDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".img") {
				continue // placeholder (.png), marker, .miss files
			}
			sharpenCover(filepath.Join(cacheDir, e.Name()))
		}
	}
	// Fresh installs land here with no directory yet: create it so the
	// marker exists before the first store-time-sharpened fetch, or the
	// next startup would re-sharpen every new file.
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return
	}
	if f, err := os.Create(filepath.Join(cacheDir, sharpenedMarker)); err == nil {
		_ = f.Close()
	}
}

// sharpenCover rewrites one cached cover to its 10% unsharp-masked form,
// keeping the original format (jpeg q90, else png) via an atomic
// temp+rename. Best-effort like normalizeCover: an undecodable or
// vanished file is left as-is.
func sharpenCover(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	src, format, err := image.Decode(f)
	_ = f.Close()
	if err != nil {
		return
	}
	b := src.Bounds()
	if b.Dx() < 2 || b.Dy() < 2 {
		return
	}
	rgba, ok := src.(*image.RGBA)
	if !ok {
		rgba = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)
	}
	out := sharpenRGBA(rgba, sharpenAmount)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sharp-*")
	if err != nil {
		return
	}
	enc := func() error {
		if format == "jpeg" {
			return jpeg.Encode(tmp, out, &jpeg.Options{Quality: 90})
		}
		return png.Encode(tmp, out)
	}
	if err := enc(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	_ = os.Rename(tmp.Name(), path)
}

// sharpenRGBA applies an unsharp mask with a 3x3 box blur (edge-replicated
// borders) to the RGB channels: out = clamp(p + amount·(p − blur)).
// Alpha passes through untouched, flat fields are exact no-ops, and the
// result is always a fresh buffer — the source is never mutated.
func sharpenRGBA(src *image.RGBA, amount float64) *image.RGBA {
	if src == nil || amount <= 0 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1, y2 := y-1, y, y+1
		if y0 < 0 {
			y0 = 0
		}
		if y2 > h-1 {
			y2 = h - 1
		}
		row0 := src.PixOffset(b.Min.X, b.Min.Y+y0)
		row1 := src.PixOffset(b.Min.X, b.Min.Y+y1)
		row2 := src.PixOffset(b.Min.X, b.Min.Y+y2)
		for x := 0; x < w; x++ {
			x0, x1, x2 := x-1, x, x+1
			if x0 < 0 {
				x0 = 0
			}
			if x2 > w-1 {
				x2 = w - 1
			}
			io := dst.PixOffset(x, y)
			is := src.PixOffset(b.Min.X+x, b.Min.Y+y)
			for c := 0; c < 3; c++ {
				sum := int(src.Pix[row0+x0*4+c]) + int(src.Pix[row0+x1*4+c]) + int(src.Pix[row0+x2*4+c]) +
					int(src.Pix[row1+x0*4+c]) + int(src.Pix[row1+x1*4+c]) + int(src.Pix[row1+x2*4+c]) +
					int(src.Pix[row2+x0*4+c]) + int(src.Pix[row2+x1*4+c]) + int(src.Pix[row2+x2*4+c])
				p := float64(src.Pix[is+c])
				v := p + amount*(p-float64(sum)/9)
				if v < 0 {
					v = 0
				} else if v > 255 {
					v = 255
				}
				dst.Pix[io+c] = uint8(v + 0.5)
			}
			dst.Pix[io+3] = src.Pix[is+3]
		}
	}
	return dst
}
