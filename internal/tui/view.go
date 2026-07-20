package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Fixed column widths for the games table; the title column takes the rest.
const (
	colPlatform = 9
	colBadges   = 17
	colVersion  = 15
	colStatus   = 15
	colGaps     = 4
)

var (
	styleMuted     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleHeader    = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true)
	styleTabActive = lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(lipgloss.Color("12"))
	styleTab       = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	styleSelected  = lipgloss.NewStyle().Reverse(true)
	styleWarn      = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleOK        = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleBusy      = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	styleTitle     = lipgloss.NewStyle().Bold(true)
	styleModal     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("11")).
			Padding(1, 3)
	styleDimmedAction = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// switchHints is the pinned footer segment advertising the number-key screen
// switches; it is never truncated so screen discovery survives narrow
// terminals.
const switchHints = "1 games · 2 settings · 3 help · 4 about"

// toneColor maps a ui badge tone hint to this frontend's palette.
func toneColor(t ui.Tone) lipgloss.Color {
	switch t {
	case ui.ToneGreen:
		return lipgloss.Color("10")
	case ui.ToneRed:
		return lipgloss.Color("9")
	case ui.ToneBlue:
		return lipgloss.Color("12")
	case ui.TonePurple:
		return lipgloss.Color("13")
	default:
		return lipgloss.Color("8")
	}
}

// trunc shortens s to w display cells, ending with an ellipsis when cut.
func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for lipgloss.Width(string(r)) > w-1 && len(r) > 0 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// cell renders s padded/truncated to exactly w cells.
func cell(s string, w int) string {
	return lipgloss.NewStyle().Width(w).Render(trunc(s, w))
}

// View renders the current session snapshot: tab bar, screen body, toasts,
// hints/input line, and the status bar.
func (m Model) View() string {
	snap := m.sess.Snapshot()
	w, h := m.width, m.height
	if w == 0 {
		w, h = 80, 24
	}

	toasts := toastLines(snap.Toasts, w)
	chrome := 3 // tab bar + hints + status
	if snap.Progress != nil {
		chrome++ // scan progress line
	}
	contentH := h - chrome - len(toasts)
	if contentH < 3 {
		contentH = 3
	}

	var body string
	if snap.Confirm != nil {
		body = lipgloss.Place(w, contentH, lipgloss.Center, lipgloss.Center, confirmBox(snap.Confirm))
	} else {
		switch m.screen {
		case screenDetail:
			body = m.detailView(w, contentH)
		case screenSettings:
			body = m.settingsView(w, contentH)
		case screenHelp:
			body = helpView()
		case screenAbout:
			body = m.aboutView()
		default:
			body = m.gamesView(snap, w, contentH)
		}
	}

	footer := m.footerView(w)
	status := m.statusView(snap, w)

	parts := []string{tabBar(m.screen), body}
	parts = append(parts, toasts...)
	if snap.Progress != nil {
		parts = append(parts, progressView(snap.Progress))
	}
	parts = append(parts, footer, status)
	// Exactly h lines: a trailing newline would push the frame to h+1 and
	// bubbletea's renderer drops the top line (the tab bar) in that case.
	return strings.Join(parts, "\n")
}

// tabBar renders the screen switcher; the active tab is accented, inactive
// tabs stay legible, and a dim separator marks the tab strip as a control.
func tabBar(active screen) string {
	tabs := []struct {
		scr   screen
		label string
	}{
		{screenGames, "1 Games"},
		{screenSettings, "2 Settings"},
		{screenHelp, "3 Help"},
		{screenAbout, "4 About"},
	}
	out := make([]string, 0, len(tabs))
	for _, t := range tabs {
		if t.scr == active || (t.scr == screenGames && active == screenDetail) {
			out = append(out, styleTabActive.Render(" "+t.label+" "))
		} else {
			out = append(out, styleTab.Render(" "+t.label+" "))
		}
	}
	return strings.Join(out, styleMuted.Render(" · "))
}

// toastLines renders up to the last three toasts; warnings are colored.
func toastLines(toasts []ui.Toast, w int) []string {
	if n := len(toasts); n > 3 {
		toasts = toasts[n-3:]
	}
	lines := make([]string, 0, len(toasts))
	for _, t := range toasts {
		line := trunc("• "+t.Text, w)
		if t.Warn {
			line = styleWarn.Render(line)
		} else {
			line = styleMuted.Render(line)
		}
		lines = append(lines, line)
	}
	return lines
}

// statusView renders the bottom status bar: spinner + busy text while an op
// runs, otherwise the session status line, plus filter/sort indicators.
func (m Model) statusView(snap ui.State, w int) string {
	left := snap.StatusLine
	if snap.Busy != "" {
		left = m.spin.View() + " " + styleBusy.Render(snap.Busy)
	}
	if snap.Query != "" {
		left += styleMuted.Render("  ·  filter: " + snap.Query)
	}
	if snap.Sort == ui.SortName {
		left += styleMuted.Render("  ·  sort: name")
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(left)
}

// progressView renders one scan-progress tick as a single line: phase
// label, done/total counters, a fixed-width rune bar, and the percentage.
func progressView(p *ui.ScanProgress) string {
	const barW = 10
	filled, pct := 0, 0
	if p.Total > 0 {
		filled = p.Done * barW / p.Total
		pct = p.Done * 100 / p.Total
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("-", barW-filled)
	return styleBusy.Render(p.Phase) +
		styleMuted.Render(fmt.Sprintf(" %d/%d ", p.Done, p.Total)) +
		"[" + bar + "]" +
		styleMuted.Render(fmt.Sprintf(" %d%%", pct))
}

// footerView renders the open text input (with its escape hint) or the
// per-screen key hints with the screen-switch hints pinned on the right.
func (m Model) footerView(w int) string {
	if m.mode != inputNone {
		return m.input.View() + styleMuted.Render("  (esc cancel · enter accept)")
	}
	var hints string
	switch m.screen {
	case screenDetail:
		hints = "i install · l launch · c cancel · r rollback · o open INI · esc back"
	case screenSettings:
		hints = "e version · t template · a add · d remove · o online info · x clear cache"
	case screenHelp, screenAbout:
		hints = "q quit"
	default:
		hints = "enter detail · i install · l launch · / filter · R rescan · s sort · q quit"
	}
	lw := w - lipgloss.Width(switchHints) - 3
	if lw < 0 {
		lw = 0
	}
	return styleMuted.Render(trunc(hints, lw) + " │ " + switchHints)
}

// confirmBox renders the pending session confirmation as a centered modal.
func confirmBox(c *ui.Confirmation) string {
	return styleModal.Render(c.Message + "\n\n" + "[y] proceed  [n] cancel" +
		"\n" + "(other keys are disabled until answered)")
}

// gamesView renders the games table: a fixed column header above a viewport
// that keeps the cursor row visible at any terminal height.
func (m Model) gamesView(snap ui.State, w, contentH int) string {
	rows := m.sess.VisibleRows()
	tw := titleWidth(w)
	header := styleHeader.Render(
		cell("TITLE", tw) + cell("STORE", colPlatform) + cell("TECH", colBadges) +
			cell("VERSION", colVersion) + cell("STATUS", colStatus))

	if len(rows) == 0 {
		var empty string
		if snap.Query != "" {
			empty = fmt.Sprintf("no games match %q (no matches) — Esc to clear the filter", snap.Query)
		} else {
			empty = "no games yet — press R to scan, or 2 → a to add a folder"
		}
		return header + "\n" + styleMuted.Render(empty)
	}

	lines := make([]string, 0, len(rows))
	for i, r := range rows {
		lines = append(lines, m.gameRowLine(r, tw, w, i == m.cursor))
	}
	vp := m.gamesVP
	vp.Width = w
	vp.Height = contentH - 1 // column header
	if vp.Height < 1 {
		vp.Height = 1
	}
	vp.SetContent(strings.Join(lines, "\n"))
	keepCursorVisible(&vp, m.cursor)
	return header + "\n" + vp.View()
}

// keepCursorVisible scrolls the viewport just enough to contain row.
func keepCursorVisible(vp *viewport.Model, row int) {
	if row < vp.YOffset {
		vp.SetYOffset(row)
	} else if row >= vp.YOffset+vp.Height {
		vp.SetYOffset(row - vp.Height + 1)
	}
}

func titleWidth(w int) int {
	tw := w - colPlatform - colBadges - colVersion - colStatus - colGaps
	if tw < 10 {
		tw = 10
	}
	return tw
}

// badgesCell composes styled tech badges into a w-cell column. Whole badges
// are accumulated while they fit (width measured on the plain text); the
// first badge that does not fit is dropped along with the rest and replaced
// by an ellipsis. Truncation happens on badge boundaries before styling, so
// ANSI sequences are never split or left unclosed.
func badgesCell(badges []ui.Badge, w int) string {
	if w <= 0 {
		return ""
	}
	var out strings.Builder
	used := 0
	cut := false
	for _, b := range badges {
		pw := lipgloss.Width("[" + b.Label + "] ")
		if used+pw > w {
			cut = true
			break
		}
		out.WriteString(lipgloss.NewStyle().Foreground(toneColor(b.Tone)).Render("["+b.Label+"]") + " ")
		used += pw
	}
	if cut {
		s := out.String()
		if used+1 > w {
			// No room for the ellipsis: swap the trailing separator space
			// (plain text, no escape risk) for the marker.
			s = strings.TrimSuffix(s, " ")
		}
		out.Reset()
		out.WriteString(s + "…")
	}
	return lipgloss.NewStyle().Width(w).Render(out.String())
}

// gameRowLine renders one games-table row; the cursor row is inverted.
func (m Model) gameRowLine(r ui.GameRow, tw, w int, selected bool) string {
	version := r.OptiScalerVersion
	if version == "" {
		version = "—"
	}
	status := string(r.Status)
	if status == "" {
		status = "not installed"
	}
	var statusCell string
	if m.sess.OpBusy(r.InstallDir) {
		statusCell = m.spin.View() + " working"
	} else {
		switch r.Status {
		case "committed":
			statusCell = styleOK.Render(status)
		case "failed", "rolled_back":
			statusCell = styleWarn.Render(status)
		case "in_progress":
			statusCell = styleBusy.Render(status)
		default:
			statusCell = styleMuted.Render(status)
		}
	}
	badges := r.TechBadges
	if r.ProtonTier != "" {
		badges = append(append([]ui.Badge(nil), badges...), ui.Badge{Label: r.ProtonTier, Tone: tierTone(r.ProtonTier)})
	}
	line := cell(r.Title, tw) +
		cell(r.Platform, colPlatform) +
		badgesCell(badges, colBadges) +
		cell(version, colVersion) +
		lipgloss.NewStyle().Width(colStatus).Render(statusCell)
	if selected {
		return styleSelected.Render(lipgloss.NewStyle().Width(w).Render(line))
	}
	return line
}

// tierTone maps a ProtonDB tier to a badge tone: better tiers read greener,
// borked reads red, unknown-ish tiers stay gray.
func tierTone(tier string) ui.Tone {
	switch tier {
	case "platinum", "gold":
		return ui.ToneGreen
	case "silver":
		return ui.ToneBlue
	case "borked":
		return ui.ToneRed
	default:
		return ui.ToneGray
	}
}

// detailView renders the selected game's metadata panel and its actions.
func (m Model) detailView(w, contentH int) string {
	row := m.detailRow()
	var content string
	if row == nil {
		content = styleMuted.Render("game no longer in the library — esc back")
	} else {
		version := row.OptiScalerVersion
		if version == "" {
			version = "not installed"
		}
		components := strings.Join(row.Components, ", ")
		if components == "" {
			components = "—"
		}
		status := string(row.Status)
		if status == "" {
			status = "not installed"
		}
		eac := "no"
		if row.EAC {
			eac = "yes"
		}
		var b strings.Builder
		b.WriteString(styleTitle.Render(row.Title) + "\n")
		fmt.Fprintf(&b, "%s · AppID %s\n", row.Platform, row.AppID)
		fmt.Fprintf(&b, "Path: %s\n", row.InstallDir)
		fmt.Fprintf(&b, "OptiScaler: %s\n", version)
		fmt.Fprintf(&b, "Components: %s\n", components)
		fmt.Fprintf(&b, "Status: %s · EAC: %s\n", status, eac)
		if row.ProtonTier != "" {
			fmt.Fprintf(&b, "ProtonDB: %s\n", row.ProtonTier)
		}
		if row.SteamAppID != "" {
			fmt.Fprintf(&b, "Steam AppID: %s\n", row.SteamAppID)
		}
		if row.CompatPrefix != "" {
			fmt.Fprintf(&b, "Proton: %s\n", row.CompatPrefix)
		}
		b.WriteString("\n" + styleHeader.Render("Actions") + "\n")
		b.WriteString("  i  install/uninstall\n")
		b.WriteString("  l  launch\n")
		b.WriteString("  c  cancel operation\n")
		rollback := "  r  rollback"
		if !row.Actionable {
			rollback = styleDimmedAction.Render(rollback + " (interrupted installs only)")
		}
		openINI := "  o  open INI"
		if row.Status != "committed" {
			openINI = styleDimmedAction.Render(openINI + " (installed games only)")
		}
		b.WriteString(rollback + "\n" + openINI + "\n")
		b.WriteString(styleMuted.Render("  esc  back"))
		content = b.String()
	}
	vp := m.detailVP
	vp.Width = w
	vp.Height = contentH
	vp.SetContent(content)
	return vp.View()
}

// settingsView renders the version/template settings above the scan-directory
// list; the list scrolls inside a viewport clamped to contentH (with the
// directory cursor kept visible) so the frame never exceeds the terminal
// height.
func (m Model) settingsView(w, contentH int) string {
	s := m.sess.Settings()
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", styleHeader.Render("Default OptiScaler version:"), s.DefaultVersion)
	fmt.Fprintf(&b, "%s %s\n", styleHeader.Render("Launch template:"), trunc(s.LaunchTemplate, w-18))
	online := "off"
	if s.OnlineLookups {
		online = "on"
	}
	fmt.Fprintf(&b, "%s %s\n", styleHeader.Render("online game info:"), online)
	b.WriteString("\n" + styleHeader.Render("Scan directories"))
	header := b.String()

	lines := make([]string, 0, len(s.ExtraDirs)+2)
	if len(s.ExtraDirs) == 0 {
		lines = append(lines, styleMuted.Render("  none yet — press a to add one"))
	}
	for i, d := range s.ExtraDirs {
		line := "  " + trunc(d, w-4)
		if i == m.dirCursor {
			line = styleSelected.Render(lipgloss.NewStyle().Width(w).Render("> " + trunc(d, w-4)))
		}
		lines = append(lines, line)
	}
	if m.confirmRmDir != "" {
		lines = append(lines, "",
			styleWarn.Render(fmt.Sprintf("remove %s? [y/n] (other keys are disabled until answered)", m.confirmRmDir)))
	}

	vp := m.settingsVP
	vp.Width = w
	vp.Height = contentH - lipgloss.Height(header)
	if vp.Height < 1 {
		vp.Height = 1
	}
	vp.SetContent(strings.Join(lines, "\n"))
	keepCursorVisible(&vp, m.dirCursor)
	return header + "\n" + vp.View()
}

// helpView renders the key reference screen.
func helpView() string {
	return styleHeader.Render("Keyboard reference") + "\n\n" + strings.Join([]string{
		"Global    1 games · 2 settings · 3 help · 4 about · q / ctrl+c quit",
		"Games     j/k move · enter detail · i install/uninstall · l launch · c cancel",
		"          / filter · R rescan · s sort",
		"Detail    i install · l launch · c cancel · r rollback · o open INI · esc back",
		"Settings  e edit version · t edit template · a add dir · d remove dir",
		"          o toggle online game info · x clear bundle cache",
		"Confirm   y proceed · n cancel",
	}, "\n")
}

// aboutView renders the version, the project tagline (mirroring the GUI
// about modal), and the TUI stack line.
func (m Model) aboutView() string {
	version := m.version
	if version == "" {
		version = "dev"
	}
	return styleTitle.Render("optiscaler-manager "+version) + "\n" +
		styleMuted.Render("OptiScaler manager for local games — Linux + Steam.") + "\n\n" +
		styleMuted.Render("TUI: bubbletea v1.3.10 · bubbles v1.0.0 · lipgloss")
}
