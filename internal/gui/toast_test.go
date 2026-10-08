package gui

import (
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Issue 058: a failed launch toasts the multi-line stderr tail; the card
// auto-sized with the wrapped text and the 40px-per-card stack math let
// the lowest toast cross the window bottom. Toasts render single-line
// (the full text lives in the log panel, issue 052) and the stack sits
// above the status bar.

func TestToastRaisedAboveStatusBarAndSingleLine(t *testing.T) {
	headlessFrames(t, 1100, 700)
	m := newModel(Config{})
	m.state.Toasts = []ui.Toast{
		{Text: "Installed Game One", AddedAt: time.Now()},
		{Text: "Launch failed: boom\nstderr two\nstderr three", Warn: true, AddedAt: time.Now()},
	}
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects

	if len(m.toastRects) != 2 {
		t.Fatalf("toast rects = %d, want 2", len(m.toastRects))
	}
	barTop := m.statusBarRect.Origin[1]
	if barTop == 0 {
		t.Fatal("status bar rect not captured")
	}
	for i, r := range m.toastRects {
		if bottom := r.Origin[1] + r.Size[1]; bottom > barTop {
			t.Errorf("toast %d bottom %v crosses the status bar top %v", i, bottom, barTop)
		}
		if r.Size[1] > 44 {
			t.Errorf("toast %d is %v px tall, want one line (~40)", i, r.Size[1])
		}
	}
}
