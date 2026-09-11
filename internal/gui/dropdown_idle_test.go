package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Dropdown menus must not churn the frame loop: after the interaction
// settles (mouse parked, no keys), idle frames report no content changes
// and no pending frame request. Sustained churn here would present as
// flicker on wayland — the app repaints every frame instead of riding the
// unchanged-frame early return.

// idleFrames runs n frames with no input and fails if the loop did not
// settle: any late frame still reporting changes or requesting another
// frame is churn.
func idleFrames(t *testing.T, view FrameFn, n int) {
	t.Helper()
	GetFrameInput().Key = KeyCodeNone
	GetInputState().Modifiers = 0
	for i := 0; i < n; i++ {
		out := RunFrameFn(view)
		if i == n-1 && out.FrameHasChanges {
			t.Errorf("idle frame %d still has content changes; the menu loop is churning", i)
		}
		if i == n-1 && out.NextFrameRequested {
			t.Errorf("idle frame %d still requests another frame; the menu loop is churning", i)
		}
	}
}

func TestVersionMenuIdleAfterOpenClose(t *testing.T) {
	sess, gameRoot := dropdownFakes(t)
	markExternal(t, gameRoot+"/bin", [4]uint16{0, 7, 0, 0})
	row := scanExternalRow(t, sess)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 400, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)

	tr := m.versionDDRects[row.InstallDir]
	if tr.Size[0] == 0 {
		t.Fatal("no version trigger rect")
	}
	GetInputState().MousePoint = Vec2{tr.Origin[0] + tr.Size[0]/2, tr.Origin[1] + tr.Size[1]/2}
	idleFrames(t, view, 3)
	clickRect(tr, view)
	idleFrames(t, view, 6) // menu open, mouse resting on the trigger

	keyFrame(KeyEscape, 0, view)
	idleFrames(t, view, 6) // closed again
	t.Log("version menu: open and closed states both settle to a quiet loop")
}

func TestDLSSMenuIdleAfterOpenClose(t *testing.T) {
	sess, _, _ := dlssGUIFakes(t, nil)
	row := scanOneRow(t, sess)
	sess.UpdateDLSS(row.InstallDir)
	waitSessEvent(t, sess, ui.EvOpDone)

	m := newModel(Config{Session: sess})
	headlessFrames(t, 400, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)

	ar := m.dlssArrowRect
	if ar.Size[0] == 0 {
		t.Fatal("no DLSS arrow rect")
	}
	GetInputState().MousePoint = Vec2{ar.Origin[0] + ar.Size[0]/2, ar.Origin[1] + ar.Size[1]/2}
	idleFrames(t, view, 3)
	clickRect(ar, view)
	idleFrames(t, view, 6) // menu open, mouse resting on the arrow

	keyFrame(KeyEscape, 0, view)
	idleFrames(t, view, 6) // closed again
	t.Log("DLSS menu: open and closed states both settle to a quiet loop")
}
