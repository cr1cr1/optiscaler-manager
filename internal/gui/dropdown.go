package gui

// The shared dropdown machinery: one trigger + popup model used by the
// version picker, the toolbar sort menu, and the DLSS restore menu. It was
// extracted from versionDropdown/sortDropdown when the DLSS restore menu
// moved onto it (it had grown its own copy of the popup, the keyboard
// navigation, and the dismissal logic).
//
// The model: the trigger is the focus owner. While the popup is open it
// consumes Up/Down (move the wrapping highlight), Enter (pick the
// highlighted row) and Space (toggle closed) via dropdownOpenKeys; the
// popup renders through Popup (root scope, escaping card Clip) anchored
// below the trigger and clamped to the window; rows adopt the highlight
// from the mouse only on actual mouse motion, so a resting mouse cannot
// fight the arrow keys; dismissal is Esc (consumed before the global
// handler) or a click outside trigger and popup.

import (
	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"
)

// dropdownState is the per-dropdown Use-hook state shared by every dropdown:
// open flag, keyboard highlight, and the trigger/popup ids the popup anchors
// to and the dismissal hit-tests against.
type dropdownState struct {
	open      bool
	btnID     ContainerId
	menuID    ContainerId
	hl        int
	prevMouse Vec2
}

// dropdownMenuKeys consumes the menu-navigation keys for a dropdown
// trigger while its popup is open: Up/Down move the highlight (wrapping),
// Enter reports a pick of the highlighted row, Space reports the
// toggle-closed gesture. Every key is consumed so no frame-end fallback
// (or the trigger's own face, for ButtonExt triggers) can also see it.
func dropdownMenuKeys(st *dropdownState, n int) (enterPick, spaceToggle bool) {
	switch GetFrameInput().Key {
	case KeyDown, KeyUp:
		if n > 0 {
			if st.hl < 0 {
				st.hl = 0
			} else if GetFrameInput().Key == KeyDown {
				st.hl = (st.hl + 1) % n
			} else {
				st.hl = (st.hl - 1 + n) % n
			}
			GetFrameInput().Key = KeyCodeNone
		}
	case KeyEnter:
		GetFrameInput().Key = KeyCodeNone
		enterPick = true
	case KeySpace:
		GetFrameInput().Key = KeyCodeNone
		spaceToggle = true
	}
	return enterPick, spaceToggle
}

// dropdownRow renders one popup row with the shared dropdown behavior: the
// row is a pointing-hand hover target (v0.17 cursor rule), hover adopts the
// highlight only when hoverAdopt is set (the popup frame's mouse-motion
// gate), the highlighted row paints the hover accent, and the row is
// picked by pointer press or by Enter when highlighted. content renders
// the row's children (and records any observability seam); pick runs after
// the row is picked and the popup is closed — keyboard distinguishes an
// Enter pick (keyboard focus never left the trigger) from a pointer pick.
func dropdownRow(st *dropdownState, idx int, enterPick, hoverAdopt bool, content func(hl bool), pick func(keyboard bool)) {
	Container(Attrs(PointerHand, Row, Expand, CrossMid, Gap(sp8), Pad2(sp4, sp8), Corners(2)), func() {
		if IsHovered() && hoverAdopt {
			st.hl = idx
		}
		hl := idx == st.hl
		if hl {
			ModAttrs(BackgroundVec(accentHov))
		}
		content(hl)
		if PressAction() || (enterPick && hl) {
			keyboard := !PressAction()
			st.open = false
			pick(keyboard)
		}
	})
}

// dropdownPopup renders the shared dark popup panel: a root-scope Popup
// (escaping the card's Clip), the panel tokens, anchored below the trigger
// via dropdownPos and clamped to the window, with st.menuID captured for
// the dismissal hit-test. minW is the panel's extra minimum width beyond
// the trigger's (the trigger width is always the floor); maxW of 0 means
// no maximum. body runs inside the panel and receives the popup frame's
// mouse-motion gate (see dropdownRow).
func dropdownPopup(st *dropdownState, minW, maxW float32, body func(mouseMoved bool)) {
	Popup(func() {
		// Mouse-motion gate, once per popup frame: hover adopts the
		// highlight only when the mouse actually moved since the last
		// popup frame. A mouse resting over a row fires IsHovered on
		// presence every frame and would overwrite the trigger's
		// arrow-key move in the same frame; a MOVED mouse still wins.
		mouseMoved := GetInputState().MousePoint != st.prevMouse
		st.prevMouse = GetInputState().MousePoint
		panel := []AttrsFn{
			MinWidth(GetResolvedRectOf(st.btnID).Size[0] + minW),
			Corners(radiusS), Pad2(sp4, 0), Gap(2), Clip,
			BackgroundVec(bgPanel), BorderWidth(1), BorderColorVec(border),
			elevateOverlay,
		}
		if maxW > 0 {
			panel = append(panel, MaxWidth(maxW))
		}
		Container(AttrsWith(Attrs(), panel...), func() {
			ModAttrs(FloatVec(dropdownPos(st.btnID)))
			st.menuID = CurrentId()
			body(mouseMoved)
		})
	})
}

// dropdownDismiss closes the dropdown on Esc — consumed so the global Esc
// handler cannot also act — or on a click outside both trigger and popup;
// it runs AFTER the popup rendered so clicks inside it still register
// (menu.go's ordering). onClosed clears whatever coordination field names
// the open dropdown.
func dropdownDismiss(st *dropdownState, onClosed func()) {
	doClose := func() {
		st.open = false
		if onClosed != nil {
			onClosed()
		}
	}
	if st.open && GetFrameInput().Key == KeyEscape {
		GetFrameInput().Key = KeyCodeNone
		doClose()
	}
	if st.open && !IdIsHovered(st.btnID) && !IdIsHovered(st.menuID) && GetFrameInput().Mouse == MouseClick {
		doClose()
	}
}

// dropdownArrow is the one dropdown affordance arrow: the larger
// sorted-down icon glyph shared by the version-dropdown trigger and the
// DLSS pill (the DLSS pill used to render a smaller text glyph of its
// own). The sort trigger's arrow is its ButtonExt icon attr — same glyph,
// same size, button-skinned.
func dropdownArrow() {
	Icon(TypArrowSortedDown, FontSize(11), TextColor(0, 0, 96, 1))
}
