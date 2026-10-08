package gui

import (
	"strings"
	"testing"
	"time"

	. "go.hasen.dev/shirei"
)

// Issue 029: the detail panel header hosts the title AND its actions —
// [title] [Set title] [Close] — wrapping so nothing leaves the visible
// area; the poster carries an icon-only Set-poster button on its own
// top-left corner.

func TestDetailHeaderHostsSetTitleLeftOfClose(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects

	if m.setTitleRect.Size[0] == 0 || m.setTitleRect.Size[1] == 0 {
		t.Fatalf("Set title button not rendered (rect %+v)", m.setTitleRect)
	}
	if m.closeBtnRect.Size[0] == 0 || m.closeBtnRect.Size[1] == 0 {
		t.Fatalf("Close button rect not captured (rect %+v)", m.closeBtnRect)
	}
	if m.panelTitleRect.Size[0] == 0 {
		t.Fatalf("title label rect not captured (rect %+v)", m.panelTitleRect)
	}
	// All three live in the header band above the poster.
	for name, r := range map[string]Rect{"title": m.panelTitleRect, "set title": m.setTitleRect, "close": m.closeBtnRect} {
		if r.Origin[1] >= m.posterRect.Origin[1] {
			t.Errorf("%s (y %.0f) is not in the header above the poster (y %.0f)",
				name, r.Origin[1], m.posterRect.Origin[1])
		}
	}
	// Order: title → Set title → Close.
	if m.panelTitleRect.Origin[0] >= m.setTitleRect.Origin[0] {
		t.Errorf("title (x %.0f) not left of Set title (x %.0f)", m.panelTitleRect.Origin[0], m.setTitleRect.Origin[0])
	}
	if m.setTitleRect.Origin[0] >= m.closeBtnRect.Origin[0] {
		t.Errorf("Set title (x %.0f) not left of Close (x %.0f)", m.setTitleRect.Origin[0], m.closeBtnRect.Origin[0])
	}
}

func TestDetailHeaderWrapsLongTitle(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	// Narrow window → minimum panel width; a long title must wrap inside
	// the header instead of pushing the buttons out of the panel.
	headlessFrames(t, 900, 800)
	keyFrame(KeyCodeNone, 0, m.rootView)
	// drain() re-snapshots m.state from the session every frame, so the
	// long title must change at the session level.
	long := strings.TrimSpace(strings.Repeat("Very Long Game Title ", 8))
	sess.SetTitleOverride(row.InstallDir, long)
	deadline := time.Now().Add(5 * time.Second)
	for sess.Snapshot().Rows[0].Title != long {
		if time.Now().After(deadline) {
			t.Fatalf("title override did not land: %q", sess.Snapshot().Rows[0].Title)
		}
		time.Sleep(10 * time.Millisecond)
	}
	keyFrame(KeyCodeNone, 0, m.rootView)

	if m.panelTitleRect.Size[1] < 30 {
		t.Errorf("long title did not wrap (title rect %+v, one line is ~22px tall)", m.panelTitleRect)
	}
	panelRight := m.detailPanelRect.Origin[0] + m.detailPanelRect.Size[0]
	for name, r := range map[string]Rect{"set title": m.setTitleRect, "close": m.closeBtnRect, "title": m.panelTitleRect} {
		if r.Size[0] == 0 {
			t.Errorf("%s not rendered in the wrapped header", name)
			continue
		}
		if right := r.Origin[0] + r.Size[0]; right > panelRight+1 {
			t.Errorf("%s (right edge %.0f) sticks out of the panel (right edge %.0f)", name, right, panelRight)
		}
	}
}

func TestDetailPosterHostsIconOnlyButton(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)

	if m.posterRect.Size[0] == 0 {
		t.Fatalf("poster rect not captured (rect %+v)", m.posterRect)
	}
	if m.posterBtnRect.Size[0] == 0 || m.posterBtnRect.Size[1] == 0 {
		t.Fatalf("poster overlay button not rendered (rect %+v)", m.posterBtnRect)
	}
	// Top-left ON the poster with the default margin.
	if dx := m.posterBtnRect.Origin[0] - m.posterRect.Origin[0]; dx < 4 || dx > 16 {
		t.Errorf("poster button x inset %.0f, want the default margin (~8)", dx)
	}
	if dy := m.posterBtnRect.Origin[1] - m.posterRect.Origin[1]; dy < 4 || dy > 16 {
		t.Errorf("poster button y inset %.0f, want the default margin (~8)", dy)
	}
	// Icon-only: roughly square, no text label width.
	if r := m.posterBtnRect; r.Size[0] > r.Size[1]*2 {
		t.Errorf("poster button %+v is wide enough to carry a text label, want icon-only", r)
	}
}

func TestDetailTitleEditorRendersInHeader(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)
	m.startTitleEdit(row)

	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)

	// While editing, the label and the Set title button are replaced by
	// the in-place editor; Close stays.
	if m.setTitleRect.Size[0] != 0 {
		t.Errorf("Set title button still rendered while editing (rect %+v)", m.setTitleRect)
	}
	if m.panelTitleRect.Size[0] != 0 {
		t.Errorf("title label still rendered while editing (rect %+v)", m.panelTitleRect)
	}
	if m.closeBtnRect.Size[0] == 0 {
		t.Error("Close button missing while editing")
	}
	if m.titleApplyRect.Size[0] == 0 {
		t.Error("title editor Apply button not rendered (rect not captured)")
	} else if m.titleApplyRect.Origin[1] >= m.posterRect.Origin[1] {
		t.Errorf("Apply (y %.0f) is not in the header above the poster (y %.0f)",
			m.titleApplyRect.Origin[1], m.posterRect.Origin[1])
	}
}

// Issue 031: Enter in the title editor's input applies, Esc cancels
// (without closing the panel); the poster overlay button is 30% larger
// and shows a "Set Poster" tooltip on hover.

func TestTitleEditorEnterApplies(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)
	m.startTitleEdit(row)

	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.titleInputID == nil {
		t.Fatal("title editor input id not captured")
	}
	FocusImmediateOn(m.titleInputID)
	m.titleBuf = "Entered Name"
	keyFrame(KeyEnter, 0, m.rootView)

	if m.titleEditDir != "" || m.titleBuf != "" {
		t.Errorf("Enter did not apply: titleEditDir=%q titleBuf=%q", m.titleEditDir, m.titleBuf)
	}
	deadline := time.Now().Add(5 * time.Second)
	for sess.Snapshot().Rows[0].Title != "Entered Name" {
		if time.Now().After(deadline) {
			t.Fatalf("applied title did not land: %q", sess.Snapshot().Rows[0].Title)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTitleEditorEscCancels(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)
	m.startTitleEdit(row)

	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	FocusImmediateOn(m.titleInputID)
	m.titleBuf = "Discarded"
	keyFrame(KeyEscape, 0, m.rootView)

	if m.titleEditDir != "" || m.titleBuf != "" {
		t.Errorf("Esc did not cancel: titleEditDir=%q titleBuf=%q", m.titleEditDir, m.titleBuf)
	}
	if got := sess.Snapshot().Rows[0].Title; got == "Discarded" {
		t.Error("Esc applied the edited title")
	}
	// Cancel, not close: the panel stays open.
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.state.Selected != row.InstallDir {
		t.Errorf("Esc closed the panel (selected %q), want cancel-only", m.state.Selected)
	}
}

func TestPosterButtonThirtyPercentLarger(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	headlessFrames(t, 1100, 1400)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)

	if m.posterBtnRect.Size[1] == 0 || m.setTitleRect.Size[1] == 0 {
		t.Fatalf("button rects missing: poster %+v, set-title %+v", m.posterBtnRect, m.setTitleRect)
	}
	ratio := m.posterBtnRect.Size[1] / m.setTitleRect.Size[1]
	if ratio < 1.15 || ratio > 1.5 {
		t.Errorf("poster button is %.2fx the default button height, want ~1.3x", ratio)
	}
}

func TestPosterButtonTooltip(t *testing.T) {
	sess, _ := guiFakes(t)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})
	sess.Select(row.InstallDir)

	headlessFrames(t, 1100, 1400)
	pillTip = pillTipState{}
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	r := m.posterBtnRect
	if r.Size[0] == 0 {
		t.Fatal("poster overlay button not rendered")
	}
	// Warm-up: fonts settle over the first hover frames, shifting rects;
	// a rect change legitimately restarts the debounce clock.
	for range 3 {
		GetInputState().MousePoint = Vec2{r.Origin[0] + r.Size[0]/2, r.Origin[1] + r.Size[1]/2}
		keyFrame(KeyCodeNone, 0, m.rootView)
		r = m.posterBtnRect
	}
	pillTip.since = time.Now().Add(-pillTipDelay)
	keyFrame(KeyCodeNone, 0, m.rootView)

	if !pillTip.shown || pillTip.text != "Set Poster" {
		t.Errorf("tooltip = (shown %v, text %q), want (true, %q)", pillTip.shown, pillTip.text, "Set Poster")
	}
}
