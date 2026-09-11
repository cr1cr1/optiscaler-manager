//go:build linux

package waylandbackend

import (
	"os"
	"strconv"

	"go.hasen.dev/shirei"
	"go.hasen.dev/shirei/internal/wayland/cursorshape"
	wos "go.hasen.dev/shirei/internal/wayland/os"
	"go.hasen.dev/shirei/internal/wayland/wl"
	"go.hasen.dev/shirei/internal/wayland/wlcursor"
)

// Wayland leaves the cursor undefined (invisible) over our surface until the
// client sets one on wl_pointer.enter. Three tiers, best first:
//
//  1. wp_cursor_shape: the compositor draws the themed cursor itself
//     (needs newer mutter; see waylandcursorshape_linux.go).
//  2. Themed xcursor: load the user's cursor theme with the vendored
//     wlcursor/xcursor loader (arm64-safe since vendoring — the upstream
//     crash was its swizzle assembly, which is gone) and hand the
//     compositor a buffer, scaled for HiDPI via set_buffer_scale.
//  3. The drawn arrow below, when no theme can be found at all.
//
// SHIREI_WL_NO_CURSOR_SHAPE=1 skips tier 1, to exercise the themed path on
// compositors that do support cursor-shape (e.g. for VM testing on Fedora).

var (
	cursorSurface *wl.Surface
	cursorBuf     *wl.Buffer
	cursorData    []byte
	cursorHotX    int32
	cursorHotY    int32
	cursorScale   int // scale the current cursor buffer was rendered at
	cursorReady   bool
)

// cursorArrow is the pointer bitmap: '#' = light outline, '.' = dark fill,
// ' ' = transparent. A dark arrow with a light edge reads on any background and
// matches typical desktop cursors. Hotspot is the tip at the top-left (0,0).
var cursorArrow = []string{
	"#         ",
	"##        ",
	"#.#       ",
	"#..#      ",
	"#...#     ",
	"#....#    ",
	"#.....#   ",
	"#......#  ",
	"#.......# ",
	"#........#",
	"#.....####",
	"#..#..#   ",
	"#.# #..#  ",
	"##  #..#  ",
	"#    #..# ",
	"     #..# ",
	"      ##  ",
}

// buildCursor (re)creates the cursor surface + buffer rendered at integer scale
// cs. Non-fatal: on failure the cursor is left to the compositor.
func buildCursor(cs int) {
	if cs < 1 {
		cs = 1
	}
	// Release a previously-built cursor buffer.
	if cursorBuf != nil {
		cursorBuf.Destroy()
		cursorBuf = nil
	}
	if cursorData != nil {
		wos.Munmap(cursorData)
		cursorData = nil
	}
	cursorReady = false

	artH := len(cursorArrow)
	artW := 0
	for _, row := range cursorArrow {
		if len(row) > artW {
			artW = len(row)
		}
	}
	w, hgt := artW*cs, artH*cs
	stride := w * 4
	size := stride * hgt

	fd, err := wos.CreateAnonymousFile(int64(size))
	if err != nil {
		perfLog("[wl] cursor file: %v", err)
		return
	}
	defer fd.Close()

	data, err := wos.Mmap(int(fd.Fd()), 0, size, wos.ProtRead|wos.ProtWrite, wos.MapShared)
	if err != nil {
		return
	}
	// ARGB8888 premultiplied (byte order B,G,R,A). Each art cell is a cs×cs block.
	put := func(px, py int, b, g, r, a byte) {
		o := py*stride + px*4
		data[o], data[o+1], data[o+2], data[o+3] = b, g, r, a
	}
	for ay, row := range cursorArrow {
		for ax := 0; ax < len(row); ax++ {
			var b, g, r, a byte
			switch row[ax] {
			case '#':
				b, g, r, a = 235, 235, 235, 255 // light outline
			case '.':
				b, g, r, a = 20, 20, 20, 255 // dark fill
			default:
				continue
			}
			for dy := 0; dy < cs; dy++ {
				for dx := 0; dx < cs; dx++ {
					put(ax*cs+dx, ay*cs+dy, b, g, r, a)
				}
			}
		}
	}

	pool, err := shm.CreatePool(fd.Fd(), int32(size))
	if err != nil {
		wos.Munmap(data)
		return
	}
	buf, err := pool.CreateBuffer(0, int32(w), int32(hgt), int32(stride), wl.ShmFormatArgb8888)
	pool.Destroy()
	if err != nil {
		wos.Munmap(data)
		return
	}

	if cursorSurface == nil {
		surf, err := compositor.CreateSurface()
		if err != nil {
			buf.Destroy()
			wos.Munmap(data)
			return
		}
		cursorSurface = surf
	}
	if compositorVer >= 3 {
		cursorSurface.SetBufferScale(int32(cs)) // buffer is cs× the logical cursor size
	}
	cursorSurface.Attach(buf, 0, 0)
	cursorSurface.Damage(0, 0, int32(artW), int32(artH)) // surface (logical) coords
	cursorSurface.Commit()

	cursorBuf = buf
	cursorData = data
	cursorHotX, cursorHotY = 0, 0 // tip at top-left, in logical coords
	cursorScale = cs
	cursorReady = true
}

var cursorShapeDisabled = os.Getenv("SHIREI_WL_NO_CURSOR_SHAPE") != ""

// PATCHED by optiscaler-manager (v0.17): the pointing-hand shape of
// wp_cursor_shape_device_v1.shape (the vendored cursorshape package only
// exports the default).
const shapePointer = 4

// handCursorNames are the xcursor names themes use for the pointing hand,
// in preference order (hand2 is the modern standard).
var handCursorNames = []string{"hand2", "hand", "pointing_hand", "pointer"}

// themed-cursor cache. The Theme is retained because the image buffers live
// in its shm pool (and os.File would close the fd when collected). PATCHED
// by optiscaler-manager (v0.17): image and tried state are keyed by
// [scale, shape] so hovering a click affordance swaps to the hand buffer;
// the theme itself stays per scale.
var (
	themedTheme = map[int]*wlcursor.Theme{}
	themedImage = map[[2]int]*wlcursor.ImageBuffer{}
	themedTried = map[[2]int]bool{}
)

// themedCursorImage loads (once per scale and shape) the user's themed
// cursor via the vendored xcursor loader: XCURSOR_THEME (or "default",
// which resolves through index.theme Inherits chains), XCURSOR_SIZE (or 24)
// times the output scale. The arrow shape loads LeftPtr as before; the
// pointing hand tries handCursorNames, falling back to the theme's default.
func themedCursorImage(cs int, shape int) *wlcursor.ImageBuffer {
	key := [2]int{cs, shape}
	if themedTried[key] {
		return themedImage[key]
	}
	themedTried[key] = true

	name := os.Getenv("XCURSOR_THEME")
	if name == "" {
		name = "default"
	}
	base := 24
	if v, err := strconv.Atoi(os.Getenv("XCURSOR_SIZE")); err == nil && v > 0 {
		base = v
	}
	size := uint32(base * cs)

	if themedTheme[cs] == nil {
		theme, err := wlcursor.LoadThemeFromName(name, size, shm)
		if err != nil {
			wlDebug("themed cursor: theme pool failed: %v", err)
			return nil
		}
		themedTheme[cs] = theme
	}
	theme := themedTheme[cs]
	var cur *wlcursor.Cursor
	var err error
	if shape == shirei.CursorShapePointer {
		for _, hand := range handCursorNames {
			if cur, err = theme.GetCursor(hand); err == nil {
				break
			}
		}
	} else {
		cur, err = theme.GetCursor(wlcursor.LeftPtr)
	}
	if err != nil {
		cur, err = theme.GetCursor("default")
	}
	if err != nil {
		wlDebug("themed cursor: no %s in theme %q: %v", cursorName(shape), name, err)
		theme.Destroy()
		themedTheme[cs] = nil
		return nil
	}
	img := cur.GetCursorImage(0)
	if img == nil {
		theme.Destroy()
		themedTheme[cs] = nil
		return nil
	}
	themedImage[key] = img
	wlDebug("themed cursor: %q size=%d -> %dx%d hotspot=(%d,%d)",
		name, size, img.GetWidth(), img.GetHeight(), img.GetHotspotX(), img.GetHotspotY())
	return img
}

// cursorName describes a shape for debug logs.
func cursorName(shape int) string {
	if shape == shirei.CursorShapePointer {
		return "hand"
	}
	return "arrow"
}

// attachThemedCursor points the cursor surface at the themed buffer and
// installs it for this enter serial. Buffer pixels are physical; the
// hotspot and damage are surface-local (logical), hence the /cs.
func attachThemedCursor(serial uint32, img *wlcursor.ImageBuffer, cs int) bool {
	if cursorSurface == nil {
		surf, err := compositor.CreateSurface()
		if err != nil {
			return false
		}
		cursorSurface = surf
	}
	if compositorVer >= 3 {
		cursorSurface.SetBufferScale(int32(cs))
	}
	cursorSurface.Attach(img.GetBuffer(), 0, 0)
	cursorSurface.Damage(0, 0, int32(img.GetWidth()/cs)+1, int32(img.GetHeight()/cs)+1)
	cursorSurface.Commit()
	pointer.SetCursor(serial, cursorSurface,
		int32(img.GetHotspotX()/cs), int32(img.GetHotspotY()/cs))
	return true
}

// PATCHED by optiscaler-manager (v0.17): cursor shape application tracking —
// the pointer serial and shape last applied, so applyPointerCursor only
// talks to the compositor when either changes.
var (
	cursorShapeSerial  uint32
	cursorShapeApplied int
)

// applyPointerCursor applies the frame's requested cursor shape; called
// after every RunFrameFn, before drawFrame's unchanged-frame early return,
// so hover changes update the cursor without a repaint.
func applyPointerCursor() {
	if pointer == nil {
		return
	}
	shape := shirei.MouseCursorShape
	if cursorShapeApplied == shape && pointerSerial == cursorShapeSerial {
		return
	}
	cursorShapeSerial = pointerSerial
	cursorShapeApplied = shape
	applyCursor(pointerSerial)
}

// applyCursor sets the cursor for this enter serial: compositor-drawn
// shape, else themed xcursor, else the drawn arrow (built lazily, rebuilt
// on scale changes). PATCHED by optiscaler-manager (v0.17): the finished
// frame's shape (shirei.MouseCursorShape) maps to the compositor shape or
// the themed hand; the drawn-bitmap tier stays the arrow for every shape.
func applyCursor(serial uint32) {
	if pointer == nil {
		return
	}
	shape := shirei.MouseCursorShape
	if !cursorShapeDisabled {
		ensureCursorShapeDevice()
		if cursorShapeDev != nil {
			if shape == shirei.CursorShapePointer {
				cursorShapeDev.SetShape(serial, shapePointer)
			} else {
				cursorShapeDev.SetShape(serial, cursorshape.ShapeDefault)
			}
			return
		}
	}
	cs := int(windowScale)
	if cs < 1 {
		cs = 1
	}
	if img := themedCursorImage(cs, shape); img != nil && attachThemedCursor(serial, img, cs) {
		return
	}
	// ponytail: drawing a hand bitmap for the theme-less fallback tier is
	// not worth it; such systems keep the arrow everywhere.
	if !cursorReady || cursorScale != cs {
		buildCursor(cs)
	}
	if cursorReady {
		pointer.SetCursor(serial, cursorSurface, cursorHotX, cursorHotY)
	}
}
