package gui

import (
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Issue 021: the 500ms tooltip debounce only elapses if frames keep coming
// while the cursor RESTS on the pill — the backend renders on input alone,
// so without a frame request the tooltip pops only when the mouse next
// moves. While the debounce is pending the overlay must request the next
// frame; once shown it must go quiet again.

func TestPillTooltipRequestsFramesWhileDebouncing(t *testing.T) {
	m := newModel(Config{})
	m.state = ui.State{Mode: ui.ViewGrid, StatusLine: "1 games", Rows: []ui.GameRow{
		{Title: "Tip Game", AppID: "2", InstallDir: "/g/tip", Platform: "Steam",
			Status: domain.StatusCommitted, OptiScalerVersion: "0.9.4",
			Components: []string{"DLSS 3.7.20"},
			TechBadges: []ui.Badge{
				{Label: "DLSS", Tone: ui.ToneGreen},
				{Label: "XeSS", Tone: ui.ToneBlue},
			}},
	}}

	headlessFrames(t, 1100, 700)
	GetInputState().MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	r := m.techPillRowRect
	if r.Size[0] == 0 {
		t.Fatal("card tech pill row not rendered; cannot hover it")
	}
	GetInputState().MousePoint = Vec2{r.Origin[0] + r.Size[0]/2, r.Origin[1] + r.Size[1]/2}
	keyFrame(KeyCodeNone, 0, m.rootView) // settle the hover rect (font shaping)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if pillTip.text == "" {
		t.Fatal("hover did not register a tooltip")
	}

	// Debounce pending, no new input: the frame must request its successor
	// or a resting cursor never reaches 500ms.
	GetHost().NextFrame.Store(false)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if !FrameRequested() {
		t.Error("debounce pending but no next frame requested: a resting cursor would never see the tooltip")
	}
	if pillTip.shown {
		t.Error("tooltip shown before the debounce elapsed")
	}

	// Debounce elapsed: the tooltip renders and the loop goes quiet.
	pillTip.since = time.Now().Add(-time.Second)
	GetHost().NextFrame.Store(false)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if !pillTip.shown {
		t.Error("tooltip not shown after the debounce elapsed")
	}
	if FrameRequested() {
		t.Error("shown tooltip still requesting frames: the idle loop would spin forever")
	}
}
