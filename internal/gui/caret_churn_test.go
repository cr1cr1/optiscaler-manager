package gui

import (
	"testing"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"
)

// A focused text field must not churn the frame loop: once the interaction
// settles (no keys, no mouse), idle frames should report no pending frame
// request. The caret blink needs a frame only at each 500ms phase flip, not
// a frame per frame — sustained churn re-presents the whole canvas (the
// backends damage/present the full surface), which shows as a subtle
// whole-window flicker while any field holds focus (issue 040).
func TestFocusedEditFieldIdleSettles(t *testing.T) {
	buf := "abc"
	_, view := editField(t, &buf) // returns with the field focused
	idleFrames(t, view, 6)
	t.Log("focused edit field settles to a quiet loop")
}

// The busy spinner is the same shape: it must advance on the shared pulse,
// not by requesting a frame per frame (issue 040).
func TestSpinnerIdleSettles(t *testing.T) {
	headlessFrames(t, 200, 60)
	view := func() {
		Container(Attrs(Viewport), func() { spinnerGlyph() })
	}
	RunFrameFn(view)
	idleFrames(t, view, 6)
	t.Log("spinner settles to a quiet loop")
}

// Time-driven widgets signal the shared pulse instead of requesting frames
// directly: a focused edit field sets pulseWanted for the frame, a blurred
// one does not, and the spinner always does (issue 040).
func TestTimeDrivenWidgetsSignalPulse(t *testing.T) {
	buf := "abc"
	_, focusedView := editField(t, &buf) // focused
	pulseWanted.Store(false)
	RunFrameFn(focusedView)
	if !pulseWanted.Load() {
		t.Error("focused edit field did not signal the pulse")
	}

	blurred := ""
	blurredView := func() {
		Container(Attrs(Viewport, Pad(8)), func() {
			themedInput(&blurred, "Search…", NoIcon, Grow(1), MinSize(140, fieldH))
		})
	}
	pulseWanted.Store(false)
	RunFrameFn(blurredView)
	if pulseWanted.Load() {
		t.Error("blurred edit field signalled the pulse")
	}

	pulseWanted.Store(false)
	RunFrameFn(func() {
		Container(Attrs(Viewport), func() { spinnerGlyph() })
	})
	if !pulseWanted.Load() {
		t.Error("spinner did not signal the pulse")
	}
}
