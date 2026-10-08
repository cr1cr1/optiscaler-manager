package gui

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "go.hasen.dev/shirei"
)

// Issue 052: the status bar stays one line tall regardless of the status
// text (multi-line umu stderr tails used to grow it vertically), and a
// Log button on its left slides up a readonly, selectable log panel.

func TestStatusBarHeightIgnoresMultilineStatus(t *testing.T) {
	headlessFrames(t, 1100, 700)
	m := newModel(Config{})
	m.state.StatusLine = "Ready"
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects
	h1 := m.statusBarRect.Size[1]
	if h1 == 0 {
		t.Fatal("status bar rect not captured")
	}

	m.state.StatusLine = "Launch failed: boom\nstderr two\nstderr three\nstderr four"
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	h2 := m.statusBarRect.Size[1]
	if h2 != h1 {
		t.Errorf("status bar grew with a multi-line status: %v -> %v", h1, h2)
	}
}

func TestLogButtonTogglesPanel(t *testing.T) {
	headlessFrames(t, 1100, 700)
	m := newModel(Config{})
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.logBtnRect.Size[0] == 0 {
		t.Fatal("Log button not rendered in the status bar")
	}
	if m.logPanelRect.Size[1] != 0 {
		t.Fatalf("log panel visible before toggle: %+v", m.logPanelRect)
	}

	m.toggleLogPanel() // open; jump the tween to its end
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.logPanelRect.Size[1] < 100 {
		t.Fatalf("log panel not open after toggle: %+v", m.logPanelRect)
	}

	m.toggleLogPanel() // close; jump the tween again
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.logPanelRect.Size[1] != 0 {
		t.Fatalf("log panel still visible after second toggle: %+v", m.logPanelRect)
	}
}

func TestEscClosesLogPanelFirst(t *testing.T) {
	headlessFrames(t, 1100, 700)
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})
	m.toggleLogPanel()
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView) // build (drains session state)
	keyFrame(KeyCodeNone, 0, m.rootView) // search field focused on open (issue 054)

	// Esc layers like the title editor (issue 031): with the search field
	// focused, Esc clears/blurs the field; the next Esc closes the panel.
	m.logSearchBuf = "abc"
	keyFrame(KeyEscape, 0, m.rootView)
	if m.logSearchBuf != "" {
		t.Error("Esc with the search field focused did not clear the buffer")
	}
	if !m.logOpen {
		t.Error("Esc with the search field focused closed the panel instead of clearing the field")
	}
	keyFrame(KeyCodeNone, 0, m.rootView) // blur settles
	keyFrame(KeyEscape, 0, m.rootView)
	if m.logOpen {
		t.Error("Esc after the blur did not close the log panel")
	}
}

func TestLogPanelShowsSessionLines(t *testing.T) {
	headlessFrames(t, 1100, 700)
	sess, _ := guiFakes(t)
	m := newModel(Config{Session: sess})

	sess.Launch("/definitely/not/a/game")
	deadline := time.Now().Add(2 * time.Second)
	for len(sess.LogLines()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(sess.LogLines()) == 0 {
		t.Fatal("session recorded no log line for the failed launch")
	}

	m.toggleLogPanel()
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView) // syncs the ring
	keyFrame(KeyCodeNone, 0, m.rootView)
	found := false
	for i := 0; i < m.logRing.Len(); i++ {
		if strings.Contains(m.logRing.Line(i), "unknown game") {
			found = true
		}
	}
	if !found {
		t.Fatalf("log ring missing the launch failure; %d lines synced", m.logRing.Len())
	}
}

func TestSyncLogLinesAppendsAndRebuilds(t *testing.T) {
	m := newModel(Config{})
	m.syncLogLines([]string{"a", "b"})
	m.syncLogLines([]string{"a", "b", "c"})
	if m.logRing.Len() != 3 {
		t.Fatalf("after appends: %d lines, want 3", m.logRing.Len())
	}
	// Shorter source (session ring evicted) forces a rebuild.
	m.syncLogLines([]string{"x", "y"})
	if m.logRing.Len() != 2 || m.logRing.Line(0) != "x" || m.logRing.Line(1) != "y" {
		t.Fatalf("rebuild wrong: %d lines, first %q", m.logRing.Len(), m.logRing.Line(0))
	}
}

// Issue 054: border, monospace, and search with hit highlight.

func TestLogSearchFindAdvancesAndWraps(t *testing.T) {
	m := newModel(Config{})
	m.syncLogLines([]string{"alpha", "beta hit", "gamma", "delta hit"})
	if got := m.logSearchFind("hit", 0); got != 1 {
		t.Fatalf("find from 0 = %d, want 1", got)
	}
	if got := m.logSearchFind("hit", 2); got != 3 {
		t.Fatalf("find from 2 = %d, want 3", got)
	}
	if got := m.logSearchFind("hit", 4); got != 1 {
		t.Fatalf("find from 4 = %d, want 1 (wrap)", got)
	}
	if got := m.logSearchFind("HIT", 0); got != 1 {
		t.Fatalf("case-insensitive find = %d, want 1", got)
	}
	if got := m.logSearchFind("zzz", 0); got != -1 {
		t.Fatalf("find miss = %d, want -1", got)
	}
	if got := m.logSearchFind("", 0); got != -1 {
		t.Fatalf("empty query = %d, want -1", got)
	}
}

func TestLogSearchScrollsMatchIntoView(t *testing.T) {
	headlessFrames(t, 1100, 700)
	m := newModel(Config{})
	lines := make([]string, 0, 200)
	for i := 0; i < 200; i++ {
		lines = append(lines, fmt.Sprintf("line %03d", i))
	}
	lines[150] = "needle here"
	m.syncLogLines(lines)
	m.logOpen = true
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // settle pin at the tail
	if m.logProbe.LastVisible < 20 {
		t.Fatalf("log view not streaming: %+v", m.logProbe)
	}

	m.logSearchBuf = "needle"
	m.logSearchNext()
	keyFrame(KeyCodeNone, 0, m.rootView) // scroll command lands
	keyFrame(KeyCodeNone, 0, m.rootView) // capture
	if m.logSearchHitLine != 150 {
		t.Errorf("hit line = %d, want 150", m.logSearchHitLine)
	}
	if m.logProbe.LastVisible < 150 {
		t.Errorf("match line 150 not rendered after search: probe %+v", m.logProbe)
	}
	if m.logProbe.Pinned {
		t.Error("tail-follow still pinned after an upward search jump")
	}
}

func TestLogPanelHeaderOrderCloseRightmost(t *testing.T) {
	headlessFrames(t, 1100, 700)
	m := newModel(Config{})
	m.syncLogLines([]string{"x"})
	m.logOpen = true
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.logSearchState == nil || m.logSearchState.boxRect.Size[0] == 0 {
		t.Fatal("search field not rendered")
	}
	field, lupe, closeB := m.logSearchState.boxRect, m.logSearchBtnRect, m.logCloseRect
	if lupe.Size[0] == 0 || closeB.Size[0] == 0 {
		t.Fatal("lupe or Close button not rendered")
	}
	if !(field.Origin[0] < lupe.Origin[0] && lupe.Origin[0] < closeB.Origin[0]) {
		t.Errorf("header order wrong: field %v lupe %v close %v", field.Origin[0], lupe.Origin[0], closeB.Origin[0])
	}
	panelRight := m.logPanelRect.Origin[0] + m.logPanelRect.Size[0]
	if closeB.Origin[0]+closeB.Size[0] > panelRight-1 {
		t.Errorf("Close not right-most: right edge %v, panel right %v", closeB.Origin[0]+closeB.Size[0], panelRight)
	}
}

func TestLogPanelBorderPainted(t *testing.T) {
	m := newModel(Config{})
	m.syncLogLines([]string{"some log line"})
	m.logOpen = true
	m.logAnimStart = time.Now().Add(-time.Second)
	out := filepath.Join(t.TempDir(), "panel.png")
	if err := renderToPNG(out, 800, 600, m.rootView); err != nil {
		t.Fatalf("renderToPNG: %v", err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// RenderToPNG emits HeadlessScale device pixels per logical point; the
	// fractional panel origin places the 1px border within a few device
	// rows of the computed edge, so scan the seam for the third color.
	x := int((m.logPanelRect.Origin[0] + m.logPanelRect.Size[0]/2) * HeadlessScale)
	y := int(m.logPanelRect.Origin[1] * HeadlessScale)
	above := img.At(x, y-4)
	inside := img.At(x, y+8)
	borderFound := false
	for dy := -2; dy <= 4; dy++ {
		if p := img.At(x, y+dy); p != above && p != inside {
			borderFound = true
		}
	}
	if !borderFound {
		t.Errorf("no visible border at panel top edge: above %v, inside %v, seam rows all match one of them", above, inside)
	}
}

// Issue 055: typing in the search field is debounced 300ms before the
// highlight query applies; Enter/lupe flushes immediately.

func TestLogSearchDebounce300ms(t *testing.T) {
	headlessFrames(t, 1100, 700)
	m := newModel(Config{})
	m.syncLogLines([]string{"alpha needle", "beta"})
	m.logOpen = true
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.logSearchQuery != "" {
		t.Fatalf("query applied before any typing: %q", m.logSearchQuery)
	}

	// A keystroke (buffer change) must NOT apply immediately.
	m.logSearchBuf = "needle"
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.logSearchQuery != "" {
		t.Fatalf("query applied without the debounce pause: %q", m.logSearchQuery)
	}
	if !m.logSearchPending {
		t.Fatal("debounce not pending after a buffer change")
	}

	// More typing inside the window re-arms the clock.
	m.logSearchBuf = "needles"
	m.logSearchDirtyAt = time.Now()
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.logSearchQuery != "" {
		t.Fatalf("query applied during continuous typing: %q", m.logSearchQuery)
	}

	// 300ms of quiet: the query promotes.
	m.logSearchDirtyAt = time.Now().Add(-400 * time.Millisecond)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if m.logSearchQuery != "needles" {
		t.Fatalf("query after the debounce pause = %q, want %q", m.logSearchQuery, "needles")
	}
	if m.logSearchPending {
		t.Error("debounce still pending after promotion")
	}
}

func TestLogSearchNextFlushesDebounce(t *testing.T) {
	headlessFrames(t, 1100, 700)
	m := newModel(Config{})
	m.syncLogLines([]string{"alpha needle", "beta"})
	m.logOpen = true
	m.logAnimStart = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)

	m.logSearchBuf = "needle"
	m.logSearchNext() // explicit search: no waiting for the 300ms pause
	if m.logSearchQuery != "needle" {
		t.Fatalf("explicit search did not flush the debounce: query %q", m.logSearchQuery)
	}
	if m.logSearchHitLine != 0 {
		t.Fatalf("hit line = %d, want 0", m.logSearchHitLine)
	}
}
