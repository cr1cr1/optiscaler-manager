package gui

// Issue 038: covers are stored on disk at full resolution (normalizeCover
// only crops to 2:3, never scales) and are scaled live at render by the
// shirei soft renderer. To counter the downscale softness, a 10% unsharp
// mask is applied once to the decoded RGBA at image-load time — via the
// vendored shirei decode hook (v0.18) — never per frame and never on disk.
// The hook is scoped to the covers cache dir so icons and other images
// pass through untouched.

import (
	"image"
	"path/filepath"
	"strings"
	"sync"

	. "go.hasen.dev/shirei"
)

// coverSharpenAmount is the unsharp-mask strength: out = p + amount·(p−blur).
const coverSharpenAmount = 0.10

var (
	coverDirsMu sync.Mutex
	coverDirs   = map[string]bool{}
	hookOnce    sync.Once
)

// installCoverSharpen registers coversDir as a sharpen scope and installs
// the shirei decode hook on first call. Called from newModel with the
// session's covers cache dir; extra calls (tests, new sessions) only widen
// the scope set.
func installCoverSharpen(coversDir string) {
	if coversDir == "" {
		return
	}
	coverDirsMu.Lock()
	coverDirs[filepath.Clean(coversDir)] = true
	coverDirsMu.Unlock()
	hookOnce.Do(func() { SetImageDecodeHook(coverDecodeHook) })
}

// coverDecodeHook sharpens decoded pixels when fpath lives under a
// registered covers dir; anything else returns nil (keep original pixels).
func coverDecodeHook(fpath string, rgba *image.RGBA) *image.RGBA {
	if !underCoverDir(fpath) {
		return nil
	}
	return sharpenRGBA(rgba, coverSharpenAmount)
}

func underCoverDir(fpath string) bool {
	coverDirsMu.Lock()
	defer coverDirsMu.Unlock()
	for d := range coverDirs {
		if rel, err := filepath.Rel(d, fpath); err == nil && rel != ".." &&
			!strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// sharpenRGBA applies an unsharp mask with a 3x3 box blur (edge-replicated
// borders) to the RGB channels: out = clamp(p + amount·(p − blur)).
// Alpha passes through untouched, flat fields are exact no-ops, and the
// result is always a fresh buffer — the source is never mutated, since
// cached ImageData pixels may be shared with render caches.
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
