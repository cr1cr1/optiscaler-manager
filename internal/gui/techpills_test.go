package gui

import (
	"testing"

	. "go.hasen.dev/shirei"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// techPills must drop tech badges that duplicate a component pill: the
// component pill ("DLSS 3.7.20") already names the tech AND its version, so
// the bare tech badge ("DLSS") is a duplicate. Non-duplicated badges stay.
func TestTechPillsDeduplicateComponentPills(t *testing.T) {
	row := ui.GameRow{
		Components: []string{"DLSS 3.7.20", "FSR 3.1"},
		TechBadges: []ui.Badge{
			{Label: "DLSS", Tone: ui.ToneGreen},
			{Label: "FSR", Tone: ui.ToneRed},
			{Label: "XeSS", Tone: ui.ToneBlue},
		},
	}
	got := techPills(&row)
	if len(got) != 1 || got[0].Label != "XeSS" {
		t.Errorf("techPills = %v, want only [XeSS] (DLSS/FSR duplicate component pills)", labelsOf(got))
	}

	// A component whose label merely SHARES A PREFIX without a boundary
	// ("DLSSG 2.1") must not drop the "DLSS" badge — different tech.
	row = ui.GameRow{
		Components: []string{"DLSSG 2.1"},
		TechBadges: []ui.Badge{{Label: "DLSS", Tone: ui.ToneGreen}, {Label: "DLSSG", Tone: ui.ToneGreen}},
	}
	got = techPills(&row)
	if len(got) != 1 || got[0].Label != "DLSS" {
		t.Errorf("techPills = %v, want only [DLSS] (DLSSG component drops DLSSG badge, not DLSS)", labelsOf(got))
	}

	// No components: every tech badge is real information.
	row = ui.GameRow{TechBadges: []ui.Badge{{Label: "DLSS", Tone: ui.ToneGreen}}}
	if got := techPills(&row); len(got) != 1 {
		t.Errorf("techPills with no components = %v, want the badge kept", labelsOf(got))
	}

	// All duplicated: nothing left.
	row = ui.GameRow{
		Components: []string{"DLSS 3.7.20"},
		TechBadges: []ui.Badge{{Label: "DLSS", Tone: ui.ToneGreen}},
	}
	if got := techPills(&row); len(got) != 0 {
		t.Errorf("techPills all duplicated = %v, want empty", labelsOf(got))
	}
}

func labelsOf(bs []ui.Badge) []string {
	out := make([]string, 0, len(bs))
	for _, b := range bs {
		out = append(out, b.Label)
	}
	return out
}

// The card must not render a tech badge row when every badge duplicates a
// component pill — that row was the visible duplication.
func TestCardSkipsFullyDuplicatedTechRow(t *testing.T) {
	m := newModel(Config{})
	m.state = ui.State{Mode: ui.ViewGrid, StatusLine: "1 games", Rows: []ui.GameRow{
		{Title: "Fork Game", AppID: "2", InstallDir: "/g/fork", Platform: "Steam",
			Status: domain.StatusCommitted, Fork: "jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass",
			OptiScalerVersion: "0.8.92", Components: []string{"DLSS 3.7.20", "FSR 3.1", "XeSS 2.0"},
			TechBadges: []ui.Badge{
				{Label: "DLSS", Tone: ui.ToneGreen},
				{Label: "FSR", Tone: ui.ToneRed},
				{Label: "XeSS", Tone: ui.ToneBlue},
			}},
	}}

	headlessFrames(t, 1100, 700)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	if r := m.techPillRowRect; r.Size[1] != 0 {
		t.Errorf("card rendered a tech badge row of fully-duplicated badges (rect %+v)", r)
	}
}

// Card and detail pane must show the SAME pills: a non-duplicated tech
// badge renders in both places.
func TestDetailPanelShowsTechBadgeRow(t *testing.T) {
	rows := []ui.GameRow{
		{Title: "Fork Game", AppID: "2", InstallDir: "/g/fork", Platform: "Steam",
			Status: domain.StatusCommitted, Fork: "jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass",
			OptiScalerVersion: "0.8.92", Components: []string{"DLSS 3.7.20"},
			TechBadges: []ui.Badge{
				{Label: "DLSS", Tone: ui.ToneGreen}, // duplicated by the component pill
				{Label: "XeSS", Tone: ui.ToneBlue},  // real info: no XeSS component
			}},
	}
	m := newModel(Config{})
	m.state = ui.State{Mode: ui.ViewGrid, StatusLine: "1 games", Rows: rows, Selected: rows[0].InstallDir}

	// 900x800: the panel sits at its minimum width (smaller cover) with a
	// taller viewport, so the tech badge row lands above the fold — below
	// the fold the virtualized panel culls it and the rect reads zero.
	headlessFrames(t, 900, 800)
	keyFrame(KeyCodeNone, 0, m.rootView)
	keyFrame(KeyCodeNone, 0, m.rootView)
	// The panel renders after the grid and captures the rect last.
	if r := m.techPillRowRect; r.Size[1] == 0 {
		t.Error("detail panel rendered no tech badge row; want parity with the card (XeSS badge)")
	}
}
