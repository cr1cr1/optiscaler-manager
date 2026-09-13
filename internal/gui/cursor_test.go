package gui

import (
	"testing"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"
)

// Mouse cursor shape selection: shirei picks the shape from the hover chain
// each frame (text entries keep the default arrow; anything clickable —
// Focusable or explicitly PointerHand — gets the pointing hand) and the
// wayland backend applies it after every frame, before the unchanged-frame
// early return (see docs/vendor-patches.md, patch v0.17). Headless tests
// assert the picked shape: RunFrameFn's hover detection always runs.

// hoverShape points the mouse at pt, runs two frames (hover detection reads
// the previous frame's hoverables), and returns the shape the frame picked.
func hoverShape(view FrameFn, pt Vec2) int {
	GetInputState().MousePoint = pt
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	return MouseCursorShape
}

// TestMouseCursorShape_HoverPicksShape: the hover-chain rule — only an
// explicit PointerHand picks the pointing hand (focusability alone does
// not: grid cards are focusable but their whole surface is not a click
// affordance); text entries and empty space keep the default arrow.
func TestMouseCursorShape_HoverPicksShape(t *testing.T) {
	headlessFrames(t, 500, 300)
	var clickable, entry, hand Rect
	view := func() {
		Container(Attrs(Viewport), func() {
			Container(Attrs(Focusable, MinSize(80, 30)), func() {
				clickable = GetScreenRectOf(CurrentId())
			})
			Container(Attrs(TextEntry, MinSize(80, 30)), func() {
				entry = GetScreenRectOf(CurrentId())
			})
			Container(Attrs(PointerHand, MinSize(80, 30)), func() {
				hand = GetScreenRectOf(CurrentId())
			})
		})
	}
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	for _, r := range []Rect{clickable, entry, hand} {
		if r.Size[0] == 0 || r.Size[1] == 0 {
			t.Fatalf("probe rect unresolved: %+v", r)
		}
	}
	center := func(r Rect) Vec2 {
		return Vec2{r.Origin[0] + r.Size[0]/2, r.Origin[1] + r.Size[1]/2}
	}

	cases := []struct {
		name string
		pt   Vec2
		want int
	}{
		// A focusable container (a grid card, a tab stop) is not a click
		// affordance: without an explicit PointerHand it keeps the arrow.
		{"focusable container", center(clickable), CursorShapeDefault},
		{"text entry", center(entry), CursorShapeDefault},
		{"pointer-hand container", center(hand), CursorShapePointer},
		{"empty space", Vec2{470, 280}, CursorShapeDefault},
	}
	for _, c := range cases {
		if got := hoverShape(view, c.pt); got != c.want {
			t.Errorf("%s: cursor shape %d, want %d", c.name, got, c.want)
		}
	}
	t.Log("hover chain picks arrow over text and empty space, hand over clickables")
}

// TestMouseCursorShape_AppClickables: the app's clickable components —
// focusable buttons and the toolbar's themed search input — pick the right
// shape in a real view: hand over the button, arrow over the text field.
func TestMouseCursorShape_AppClickables(t *testing.T) {
	headlessFrames(t, 400, 200)
	buf := ""
	var btn, field Rect
	view := func() {
		Container(Attrs(Viewport), func() {
			focusableButton(NoIcon, "Go")
			btn = GetScreenRectOf(GetLastId())
			themedInput(&buf, "latest", NoIcon, MinSize(160, 26))
			field = GetScreenRectOf(GetLastId())
		})
	}
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if btn.Size[0] == 0 || field.Size[0] == 0 {
		t.Fatalf("probe rects unresolved: btn %+v field %+v", btn, field)
	}
	center := func(r Rect) Vec2 {
		return Vec2{r.Origin[0] + r.Size[0]/2, r.Origin[1] + r.Size[1]/2}
	}
	if got := hoverShape(view, center(btn)); got != CursorShapePointer {
		t.Errorf("focusable button: cursor shape %d, want pointer", got)
	}
	if got := hoverShape(view, center(field)); got != CursorShapeDefault {
		t.Errorf("themed input: cursor shape %d, want default (text entry)", got)
	}
	t.Log("app clickables pick hand over buttons, arrow over text fields")
}

// TestMouseCursorShape_AppChrome: hover affordances that are not focusable
// on their own — sidebar items and view-switch segments — also pick the
// pointing hand via the explicit PointerHand attr.
func TestMouseCursorShape_AppChrome(t *testing.T) {
	m := newModel(Config{})
	headlessFrames(t, 800, 600)
	sidebar := func() {
		Container(Attrs(Viewport, Row), func() {
			m.sidebar()
		})
	}
	keyFrame(KeyCodeNone, 0, sidebar)
	keyFrame(KeyCodeNone, 0, sidebar)
	if len(m.sidebarRects) == 0 {
		t.Fatal("sidebar item rects not captured")
	}
	r := m.sidebarRects[0]
	pt := Vec2{r.Origin[0] + r.Size[0]/2, r.Origin[1] + r.Size[1]/2}
	if got := hoverShape(sidebar, pt); got != CursorShapePointer {
		t.Errorf("sidebar item: cursor shape %d, want pointer", got)
	}

	_, vm := viewSwitchPopulated(t)
	switcher := func() {
		Container(Attrs(Viewport), func() {
			vm.viewSwitch()
		})
	}
	keyFrame(KeyCodeNone, 0, switcher)
	keyFrame(KeyCodeNone, 0, switcher)
	if vm.listSegRect.Size[0] == 0 {
		t.Fatal("list segment rect not captured")
	}
	pt = Vec2{vm.listSegRect.Origin[0] + vm.listSegRect.Size[0]/2, vm.listSegRect.Origin[1] + vm.listSegRect.Size[1]/2}
	if got := hoverShape(switcher, pt); got != CursorShapePointer {
		t.Errorf("view-switch segment: cursor shape %d, want pointer", got)
	}
	t.Log("sidebar items and view-switch segments pick the pointing hand")
}

// TestMouseCursorShape_DLSSPill: the pill the request names directly — the
// DLSS pill's arrow segment is a clickable component, so it picks the
// pointing hand in the real card view.
func TestMouseCursorShape_DLSSPill(t *testing.T) {
	sess, _, _ := dlssGUIFakes(t, nil)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	headlessFrames(t, 400, 800)
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	if m.dlssArrowRect.Size[0] == 0 {
		t.Fatal("no DLSS arrow rect")
	}
	pt := Vec2{m.dlssArrowRect.Origin[0] + m.dlssArrowRect.Size[0]/2, m.dlssArrowRect.Origin[1] + m.dlssArrowRect.Size[1]/2}
	if got := hoverShape(view, pt); got != CursorShapePointer {
		t.Errorf("DLSS pill arrow: cursor shape %d, want pointer", got)
	}
	t.Log("DLSS pill arrow picks the pointing hand")
}
