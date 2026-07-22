package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
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

// sgrBefore returns the last SGR sequence in s that starts before the first
// occurrence of substr ("" when none precedes it) — the style a rendered
// segment actually wears.
func sgrBefore(s, substr string) string {
	idx := strings.Index(s, substr)
	if idx < 0 {
		return ""
	}
	last := ""
	for _, m := range sgrRE.FindAllStringIndex(s, -1) {
		if m[0] >= idx {
			break
		}
		last = s[m[0]:m[1]]
	}
	return last
}

// detailModelFor boots a session warm from a one-row games cache and binds
// a detail-screen model to that row.
func detailModelFor(t *testing.T, row ui.GameRow) Model {
	t.Helper()
	settingsDir := t.TempDir()
	e := newTestEnv(t, func(d *ui.Deps) { d.SettingsRoot = settingsDir })
	seedGamesCache(t, settingsDir, []ui.GameRow{row})
	e.sess.Start(context.Background())
	return Model{sess: e.sess, screen: screenDetail, detailDir: row.InstallDir}
}

// TestGameRowLineExternalStatus: an external install (OptiScaler on disk,
// not manager-made) renders its status with a distinct accent — not the
// committed green, warn red, busy yellow, or muted gray — while keeping the
// row ANSI-safe and exactly one table width wide.
func TestGameRowLineExternalStatus(t *testing.T) {
	forceANSI(t)

	e := newTestEnv(t, nil)
	m := Model{sess: e.sess}
	row := ui.GameRow{
		Title:    "External Game",
		Platform: "Steam",
		Status:   domain.StatusExternal,
	}

	line := m.gameRowLine(row, 20, 80, false)
	t.Logf("rendered external row: %q", line)

	if rest := sgrRE.ReplaceAllString(line, ""); strings.Contains(rest, "\x1b") {
		t.Errorf("truncated escape sequence in rendered row: %q", line)
	}
	if !strings.Contains(sgrRE.ReplaceAllString(line, ""), "external") {
		t.Errorf("external status text missing: %q", line)
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

	// The accent must be distinct from every existing status style.
	seq := sgrBefore(line, "external")
	banned := []string{"",
		sgrRE.FindString(styleOK.Render("x")),
		sgrRE.FindString(styleWarn.Render("x")),
		sgrRE.FindString(styleBusy.Render("x")),
		sgrRE.FindString(styleMuted.Render("x")),
	}
	for _, b := range banned {
		if seq == b {
			t.Errorf("external status styled with %q; want a distinct accent (not committed/warn/busy/muted)", seq)
		}
	}
}

// TestDetailViewOpenININotDimmedForExternal: the open-INI action stays live
// for external installs (CanOpenINI covers committed and external) and keeps
// its dimmed gating only for rows with no usable ini.
func TestDetailViewOpenININotDimmedForExternal(t *testing.T) {
	forceANSI(t)
	dimSeq := sgrRE.FindString(styleDimmedAction.Render("x"))

	base := ui.GameRow{
		Title:        "Game One",
		AppID:        "100",
		InstallDir:   "/games/one",
		InjectionDir: "/games/one/bin",
		Platform:     "Steam",
	}

	t.Run("external", func(t *testing.T) {
		row := base
		row.Status = domain.StatusExternal
		out := detailModelFor(t, row).detailView(100, 40)
		t.Logf("external detail view:\n%s", out)

		plain := sgrRE.ReplaceAllString(out, "")
		if !strings.Contains(plain, "open INI") {
			t.Fatalf("open INI action missing: %q", plain)
		}
		if got := sgrBefore(out, "open INI"); got == dimSeq {
			t.Errorf("open INI dimmed for an external row (seq %q); external installs have a usable ini", got)
		}
		if strings.Contains(plain, "(installed games only)") {
			t.Errorf("external row shows the gated-action suffix: %q", plain)
		}
	})

	t.Run("not installed stays dimmed", func(t *testing.T) {
		row := base
		row.InjectionDir = ""
		out := detailModelFor(t, row).detailView(100, 40)
		t.Logf("not-installed detail view:\n%s", out)

		plain := sgrRE.ReplaceAllString(out, "")
		if got := sgrBefore(out, "open INI"); got != dimSeq {
			t.Errorf("open INI not dimmed for a never-installed row (seq %q, want %q)", got, dimSeq)
		}
		if !strings.Contains(plain, "(installed games only)") {
			t.Errorf("gated-action suffix missing for a never-installed row: %q", plain)
		}
	})
}

// TestDetailViewUpgradeHint: an eligible row advertises the upgrade on
// the i action with its concrete target — the TUI equivalent of the
// GUI's "Upgrade to X" button — overriding both the install/uninstall
// and the external-adopt captions. Drives the real session flow (the
// games cache strips offers on load, so seeding one can't work).
func TestDetailViewUpgradeHint(t *testing.T) {
	ctx := context.Background()

	t.Run("committed", func(t *testing.T) {
		e := newTestEnv(t, func(d *ui.Deps) {
			d.Settings = settings.Settings{DefaultVersion: "v0.9.4-test", OnlineLookups: true}
		})
		e.sess.Start(ctx)
		e.sess.Scan(ctx)
		pollUntil(t, "initial scan", func() bool { return len(e.sess.Snapshot().Rows) == 1 })
		e.sess.QuickInstall(e.gameRoot)
		pollUntil(t, "install", func() bool {
			rows := e.sess.Snapshot().Rows
			return len(rows) == 1 && rows[0].Status == domain.StatusCommitted
		})

		e.sess.SetDefaultVersion("v0.10.0-test")
		e.sess.Scan(ctx)
		pollUntil(t, "upgrade offer", func() bool {
			rows := e.sess.Snapshot().Rows
			return len(rows) == 1 && rows[0].UpgradeAvailable && rows[0].UpgradeTarget == "v0.10.0-test"
		})

		m := Model{sess: e.sess, screen: screenDetail, detailDir: e.gameRoot}
		plain := sgrRE.ReplaceAllString(m.detailView(100, 40), "")
		if !strings.Contains(plain, "upgrade to v0.10.0-test") {
			t.Fatalf("upgrade hint missing: %q", plain)
		}
	})

	t.Run("external", func(t *testing.T) {
		e := newTestEnv(t, func(d *ui.Deps) {
			d.Settings = settings.Settings{DefaultVersion: "v0.10.0-test", OnlineLookups: true}
		})
		marker := testutil.StringInfoPE(false, map[string]string{
			"ProductName":      "OptiScaler",
			"OriginalFilename": "OptiScaler.dll",
		}, [4]uint16{0, 7, 0, 0})
		writeFile(t, filepath.Join(e.bin, "dxgi.dll"), string(marker))
		e.sess.Start(ctx)
		e.sess.Scan(ctx)
		pollUntil(t, "external upgrade offer", func() bool {
			rows := e.sess.Snapshot().Rows
			return len(rows) == 1 && rows[0].Status == domain.StatusExternal &&
				rows[0].UpgradeAvailable && rows[0].UpgradeTarget == "v0.10.0-test"
		})

		m := Model{sess: e.sess, screen: screenDetail, detailDir: e.gameRoot}
		plain := sgrRE.ReplaceAllString(m.detailView(100, 40), "")
		if !strings.Contains(plain, "upgrade to v0.10.0-test") {
			t.Fatalf("upgrade hint missing for external row: %q", plain)
		}
		if strings.Contains(plain, "adopt") {
			t.Errorf("external upgrade still shows the adopt caption: %q", plain)
		}
	})
}

// TestGameRowLineUpgradeBadge: the games list surfaces an available
// upgrade as a badge with the concrete target, so the offer is visible
// without opening the detail screen. The badge leads the cell: on
// badge-heavy rows (tech badges + ProtonDB tier) a trailing badge would
// be truncated into the ellipsis and the offer would never be seen.
func TestGameRowLineUpgradeBadge(t *testing.T) {
	forceANSI(t)

	e := newTestEnv(t, nil)
	m := Model{sess: e.sess}
	row := ui.GameRow{
		Title:             "Old Game",
		Platform:          "Steam",
		Status:            domain.StatusCommitted,
		OptiScalerVersion: "v0.9.4-test",
		TechBadges:        []ui.Badge{{Label: "DLSS", Tone: ui.ToneGreen}},
		UpgradeAvailable:  true,
		UpgradeTarget:     "v1.0.0",
	}

	line := m.gameRowLine(row, 20, 80, false)
	t.Logf("rendered upgrade row: %q", line)
	plain := sgrRE.ReplaceAllString(line, "")
	if !strings.Contains(plain, "↑v1.0.0") {
		t.Fatalf("upgrade target badge missing: %q", plain)
	}
	if !strings.Contains(plain, "DLSS") {
		t.Fatalf("test fixture overflowed the badges cell; shorten it: %q", plain)
	}
	if strings.Index(plain, "↑v1.0.0") > strings.Index(plain, "DLSS") {
		t.Errorf("upgrade badge trails the tech badges (truncation eats it on busy rows): %q", plain)
	}
	if rest := plain; strings.Contains(rest, "\x1b") {
		t.Errorf("truncated escape sequence in rendered row: %q", line)
	}
}

// TestDetailViewAdoptHintForExternal: installing over an external install is
// an adoption, so the i action says so; committed rows keep the
// install/uninstall wording.
func TestDetailViewAdoptHintForExternal(t *testing.T) {
	base := ui.GameRow{
		Title:        "Game One",
		AppID:        "100",
		InstallDir:   "/games/one",
		InjectionDir: "/games/one/bin",
		Platform:     "Steam",
	}

	t.Run("external", func(t *testing.T) {
		row := base
		row.Status = domain.StatusExternal
		out := detailModelFor(t, row).detailView(100, 40)
		plain := sgrRE.ReplaceAllString(out, "")
		t.Logf("external detail actions:\n%s", plain)

		if !strings.Contains(plain, "adopt (install over external)") {
			t.Errorf("adopt hint missing for an external row: %q", plain)
		}
		if strings.Contains(plain, "install/uninstall") {
			t.Errorf("external rows must not promise uninstall of a foreign install: %q", plain)
		}
	})

	t.Run("committed keeps install/uninstall", func(t *testing.T) {
		row := base
		row.Status = domain.StatusCommitted
		out := detailModelFor(t, row).detailView(100, 40)
		plain := sgrRE.ReplaceAllString(out, "")

		if !strings.Contains(plain, "install/uninstall") {
			t.Errorf("committed row lost the install/uninstall action: %q", plain)
		}
		if strings.Contains(plain, "adopt") {
			t.Errorf("committed row shows the adopt hint: %q", plain)
		}
	})
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

// TestProgressViewRendersPhaseBarAndPercent: mid-scan progress renders the
// phase label, done/total counters, a rune bar, and the percentage.
func TestProgressViewRendersPhaseBarAndPercent(t *testing.T) {
	line := progressView(&ui.ScanProgress{Phase: "enrich", Done: 6, Total: 10})
	t.Logf("progress line: %q", line)
	plain := sgrRE.ReplaceAllString(line, "")
	for _, want := range []string{"enrich", "6/10", "[██████----]", "60%"} {
		if !strings.Contains(plain, want) {
			t.Errorf("progress line lacks %q: %q", want, plain)
		}
	}
}

// TestTUISettingsViewRespectsContentHeight: with more scan directories than
// the terminal has rows, the settings body must be clamped to contentH so
// the full frame stays exactly h lines — an oversized frame makes
// bubbletea's renderer drop the top line (the tab bar).
func TestTUISettingsViewRespectsContentHeight(t *testing.T) {
	dirs := make([]string, 0, 40)
	for i := 1; i <= 40; i++ {
		dirs = append(dirs, fmt.Sprintf("/games/dir%02d", i))
	}
	e := newTestEnv(t, func(d *ui.Deps) { d.Settings.ExtraDirs = dirs })
	m := Model{sess: e.sess, screen: screenSettings, width: 80, height: 24}

	frame := m.View()
	lines := strings.Split(frame, "\n")
	t.Logf("settings frame (%d lines):\n%s", len(lines), frame)
	if len(lines) != 24 {
		t.Errorf("frame has %d lines, want exactly 24", len(lines))
	}
	if !strings.Contains(lines[0], "1 Games") {
		t.Errorf("first line lacks the tab bar: %q", lines[0])
	}
}

// TestProgressViewZeroTotalNoPanic: a zero total (unknown phase size) renders
// an empty bar at 0% instead of dividing by zero.
func TestProgressViewZeroTotalNoPanic(t *testing.T) {
	line := progressView(&ui.ScanProgress{Phase: "discover", Done: 0, Total: 0})
	plain := sgrRE.ReplaceAllString(line, "")
	if !strings.Contains(plain, "[----------]") || !strings.Contains(plain, "0%") {
		t.Errorf("zero-total progress line wrong: %q", plain)
	}
}
