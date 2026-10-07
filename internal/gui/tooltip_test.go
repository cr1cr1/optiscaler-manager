package gui

import (
	"testing"
	"time"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Issue 019: hovering a pill shows a tooltip with the full name of the
// label's abbreviation (DLSS → NVIDIA Deep Learning Super Sampling, …),
// debounced by 500ms so sweeping across a pill row doesn't strobe overlays.

// TestPillTipText maps pill labels to their abbreviation's full name:
// version suffixes are stripped ("DLSS 3.7.20" → DLSS), unknown labels and
// plain names get no tooltip.
func TestPillTipText(t *testing.T) {
	cases := []struct {
		label string
		want  string
	}{
		{"DLSS", "NVIDIA Deep Learning Super Sampling"},
		{"DLSS 3.7.20", "NVIDIA Deep Learning Super Sampling"},
		{"DLSS-FG", "NVIDIA DLSS Frame Generation"},
		{"FSR", "AMD FidelityFX Super Resolution"},
		{"FSR 3.1", "AMD FidelityFX Super Resolution"},
		{"XeSS 2.0", "Intel Xe Super Sampling"},
		{"EAC", "Easy Anti-Cheat"},
		// Not abbreviations / unknown: no tooltip.
		{"Proton", ""},
		{"Steam", ""},
		{"✦ OptiScaler 0.9.4", ""},
		{"DLSSG 2.1", ""}, // shares a prefix with DLSS but is not it
		{"working", ""},
	}
	for _, c := range cases {
		if got := pillTipText(c.label); got != c.want {
			t.Errorf("pillTipText(%q) = %q, want %q", c.label, got, c.want)
		}
	}
}

// Hovering a tech badge pill on a card registers the tooltip and shows it
// once the 500ms debounce has elapsed; moving the mouse away clears it and
// re-hovering debounces from zero again. The card's tech row here holds a
// single XeSS pill (the DLSS badge is deduped by the DLSS component pill),
// so the row rect IS the pill rect.
func TestCardTechBadgeHoverShowsTooltip(t *testing.T) {
	m := newModel(Config{})
	m.state = ui.State{Mode: ui.ViewGrid, StatusLine: "1 games", Rows: []ui.GameRow{
		{Title: "Tip Game", AppID: "2", InstallDir: "/g/tip", Platform: "Steam",
			Status: domain.StatusCommitted, OptiScalerVersion: "0.9.4",
			Components: []string{"DLSS 3.7.20"},
			TechBadges: []ui.Badge{
				{Label: "DLSS", Tone: ui.ToneGreen}, // deduped by the component pill
				{Label: "XeSS", Tone: ui.ToneBlue},
			}},
	}}

	headlessFrames(t, 1100, 700)
	GetInputState().MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView) // build
	keyFrame(KeyCodeNone, 0, m.rootView) // capture rects
	r := m.techPillRowRect
	if r.Size[0] == 0 {
		t.Fatal("card tech pill row not rendered; cannot hover it")
	}
	if pillTip.text != "" {
		t.Fatalf("tooltip %q with mouse parked outside, want none", pillTip.text)
	}

	hover := Vec2{r.Origin[0] + r.Size[0]/2, r.Origin[1] + r.Size[1]/2}
	GetInputState().MousePoint = hover
	// Warm-up hover frames: the first hover frames can still reshape text
	// (font init), shifting the pill rect — and a changed rect restarts the
	// debounce clock. Settle the rect before asserting on the clock.
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if pillTip.text != "Intel Xe Super Sampling" {
		t.Fatalf("hover tooltip = %q, want %q", pillTip.text, "Intel Xe Super Sampling")
	}
	if pillTip.rect.Size[0] == 0 {
		t.Fatal("hover tooltip has no anchor rect")
	}
	if pillTip.shown {
		t.Error("tooltip rendered before the 500ms debounce elapsed")
	}

	// Same pill, debounce elapsed (backdated clock): the overlay renders.
	pillTip.since = time.Now().Add(-time.Second)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if !pillTip.shown {
		t.Error("tooltip not rendered after the debounce elapsed")
	}

	// Mouse away: cleared. Re-hovering debounces from zero again.
	GetInputState().MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView)
	if pillTip.text != "" {
		t.Fatalf("tooltip %q after mouse left, want cleared", pillTip.text)
	}
	GetInputState().MousePoint = hover
	keyFrame(KeyCodeNone, 0, m.rootView)
	if pillTip.text == "" || pillTip.shown {
		t.Errorf("re-hover: text %q shown %v, want registered but debounced", pillTip.text, pillTip.shown)
	}
}

// The DLSS component pill is the interactive update/restore control (not a
// plain badgePill), but hovering it must register the DLSS tooltip too.
func TestCardDLSSControlHoverShowsTooltip(t *testing.T) {
	sess, _, _ := dlssGUIFakes(t, nil)
	row := scanOneRow(t, sess)
	m := newModel(Config{Session: sess})

	headlessFrames(t, 400, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	view := cardView(m, row)
	keyFrame(KeyCodeNone, 0, view)
	keyFrame(KeyCodeNone, 0, view)
	r := m.dlssUpdateRect
	if r.Size[0] == 0 {
		t.Fatal("DLSS control not rendered on card; cannot hover it")
	}

	pillTip = pillTipState{} // cardView is a partial view: no rootView re-arm
	GetInputState().MousePoint = Vec2{r.Origin[0] + r.Size[0]/2, r.Origin[1] + r.Size[1]/2}
	keyFrame(KeyCodeNone, 0, view)
	if pillTip.text != "NVIDIA Deep Learning Super Sampling" {
		t.Errorf("DLSS control hover tooltip = %q, want %q", pillTip.text, "NVIDIA Deep Learning Super Sampling")
	}
}

// The detail pane's pills carry the same tooltips as the card's.
func TestDetailPanelTechBadgeHoverShowsTooltip(t *testing.T) {
	row := ui.GameRow{Title: "Tip Game", AppID: "2", InstallDir: "/g/tip", Platform: "Steam",
		Status: domain.StatusCommitted, OptiScalerVersion: "0.9.4",
		Components: []string{"DLSS 3.7.20"},
		TechBadges: []ui.Badge{
			{Label: "DLSS", Tone: ui.ToneGreen},
			{Label: "XeSS", Tone: ui.ToneBlue},
		}}
	m := newModel(Config{})
	m.state = ui.State{Mode: ui.ViewGrid, StatusLine: "1 games", Rows: []ui.GameRow{row}, Selected: row.InstallDir}

	// 900x800 keeps the panel's tech row above the virtualized fold.
	headlessFrames(t, 900, 800)
	GetInputState().MousePoint = Vec2{-50, -50}
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	r := m.techPillRowRect // the panel renders last and owns the seam
	if r.Size[0] == 0 {
		t.Fatal("panel tech pill row not rendered; cannot hover it")
	}

	GetInputState().MousePoint = Vec2{r.Origin[0] + r.Size[0]/2, r.Origin[1] + r.Size[1]/2}
	keyFrame(KeyCodeNone, 0, m.rootView)
	if pillTip.text != "Intel Xe Super Sampling" {
		t.Errorf("panel hover tooltip = %q, want %q", pillTip.text, "Intel Xe Super Sampling")
	}
}
