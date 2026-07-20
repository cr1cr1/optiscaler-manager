package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// sgrRE matches one complete SGR (Select Graphic Rendition) sequence.
var sgrRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// forceANSI pins lipgloss to the ANSI256 profile so styled rendering always
// emits escape sequences, regardless of the test terminal.
func forceANSI(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// TestGameRowLineBadgesANSIIntact renders a row whose styled tech badges
// overflow the badges column and asserts truncation keeps every ANSI
// sequence complete and every color open paired with a reset. Before the
// badge-aware fix, trunc() measured width ANSI-aware but sliced by runes,
// stripping closing resets (color bleed) or cutting mid-escape (garbage).
func TestGameRowLineBadgesANSIIntact(t *testing.T) {
	forceANSI(t)

	e := newTestEnv(t, nil)
	m := Model{sess: e.sess}
	row := ui.GameRow{
		Title:    "Badge Game",
		Platform: "Steam",
		Status:   "committed",
		TechBadges: []ui.Badge{
			{Label: "DLSS 3.7", Tone: ui.ToneGreen},
			{Label: "FSR 3.1", Tone: ui.ToneRed},
			{Label: "XeSS 2.0", Tone: ui.ToneBlue},
		},
	}

	line := m.gameRowLine(row, 20, 80, false)
	t.Logf("rendered row: %q", line)

	if rest := sgrRE.ReplaceAllString(line, ""); strings.Contains(rest, "\x1b") {
		t.Errorf("truncated escape sequence in rendered row: %q", line)
	}

	opens, resets := 0, 0
	for _, seq := range sgrRE.FindAllString(line, -1) {
		if seq == "\x1b[0m" {
			resets++
		} else {
			opens++
		}
	}
	if opens != resets {
		t.Errorf("unbalanced SGR in rendered row: %d opens, %d resets: %q", opens, resets, line)
	}

	want := 20 + colPlatform + colBadges + colVersion + colStatus
	if w := lipgloss.Width(line); w != want {
		t.Errorf("row width = %d cells, want %d: %q", w, want, line)
	}
}

// TestBadgesCellDropsWholeBadges: overflowing badges are dropped whole (with
// an ellipsis marker) instead of being sliced mid-badge.
func TestBadgesCellDropsWholeBadges(t *testing.T) {
	forceANSI(t)

	badges := []ui.Badge{
		{Label: "DLSS 3.7", Tone: ui.ToneGreen},
		{Label: "FSR 3.1", Tone: ui.ToneRed},
		{Label: "XeSS 2.0", Tone: ui.ToneBlue},
	}
	cellOut := badgesCell(badges, colBadges)
	t.Logf("badges cell: %q", cellOut)

	plain := sgrRE.ReplaceAllString(cellOut, "")
	if !strings.Contains(plain, "[DLSS 3.7]") {
		t.Errorf("first badge missing: %q", cellOut)
	}
	if strings.Contains(plain, "[XeSS") {
		t.Errorf("overflowing badge partially rendered: %q", cellOut)
	}
	if !strings.Contains(plain, "…") {
		t.Errorf("ellipsis marker missing: %q", cellOut)
	}
	if w := lipgloss.Width(cellOut); w != colBadges {
		t.Errorf("badges cell width = %d, want %d: %q", w, colBadges, cellOut)
	}
}

// TestBadgesCellFitsAll: badges within budget render in full, no ellipsis.
func TestBadgesCellFitsAll(t *testing.T) {
	forceANSI(t)

	badges := []ui.Badge{
		{Label: "DLSS", Tone: ui.ToneGreen},
		{Label: "FSR", Tone: ui.ToneRed},
	}
	cellOut := badgesCell(badges, colBadges)
	t.Logf("badges cell: %q", cellOut)

	plain := sgrRE.ReplaceAllString(cellOut, "")
	if !strings.Contains(plain, "[DLSS]") || !strings.Contains(plain, "[FSR]") {
		t.Errorf("badges missing: %q", cellOut)
	}
	if strings.Contains(plain, "…") {
		t.Errorf("unexpected ellipsis when everything fits: %q", cellOut)
	}
}

// TestDetailRowMatchesSnapshotRow: detailRow returns the row whose
// InstallDir matches detailDir, read from a single snapshot.
func TestDetailRowMatchesSnapshotRow(t *testing.T) {
	e := newTestEnv(t, nil)
	m := Model{sess: e.sess, detailDir: "/no/such/dir"}
	if row := m.detailRow(); row != nil {
		t.Errorf("detailRow with unknown dir = %+v, want nil", row)
	}
}
