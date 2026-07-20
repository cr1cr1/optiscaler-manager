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
	styleTabActive = lipgloss.NewStyle().Bold(true).Reverse(true)
	styleTab       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
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
	contentH := h - 3 - len(toasts) // tab bar + hints + status
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
			body = m.settingsView(w)
		case screenHelp:
			body = helpView()
		default:
			body = m.gamesView(snap, w, contentH)
		}
	}

	footer := m.footerView(w)
	status := m.statusView(snap, w)

	parts := []string{tabBar(m.screen), body}
	parts = append(parts, toasts...)
	parts = append(parts, footer, status)
	return strings.Join(parts, "\n") + "\n"
}

// tabBar renders the screen switcher; the active tab is highlighted.
func tabBar(active screen) string {
	tabs := []struct {
		scr   screen
		label string
	}{
		{screenGames, "1 Games"},
		{screenSettings, "2 Settings"},
		{screenHelp, "3 Help"},
	}
	out := make([]string, 0, len(tabs))
	for _, t := range tabs {
		if t.scr == active || (t.scr == screenGames && active == screenDetail) {
			out = append(out, styleTabActive.Render(" "+t.label+" "))
		} else {
			out = append(out, styleTab.Render(" "+t.label+" "))
		}
	}
	return strings.Join(out, "  ")
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

// footerView renders the open text input or the per-screen key hints.
func (m Model) footerView(w int) string {
	if m.mode != inputNone {
		return m.input.View()
	}
	var hints string
	switch m.screen {
	case screenDetail:
		hints = "i install · l launch · c cancel · r rollback · o open INI · esc back"
	case screenSettings:
		hints = "e version · t template · a add · d remove · x clear cache · 1 games"
	case screenHelp:
		hints = "1 games · 2 settings · q quit"
	default:
		hints = "enter detail · i install · l launch · / filter · R rescan · s sort · q quit"
	}
	return styleMuted.Render(trunc(hints, w))
}

// confirmBox renders the pending session confirmation as a centered modal.
func confirmBox(c *ui.Confirmation) string {
	return styleModal.Render(c.Message + "\n\n" + "[y] proceed  [n] cancel")
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
	line := cell(r.Title, tw) +
		cell(r.Platform, colPlatform) +
		badgesCell(r.TechBadges, colBadges) +
		cell(version, colVersion) +
		lipgloss.NewStyle().Width(colStatus).Render(statusCell)
	if selected {
		return styleSelected.Render(lipgloss.NewStyle().Width(w).Render(line))
	}
	return line
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

// settingsView renders the version/template settings and the scan-directory
// list with its inline add/remove affordances.
func (m Model) settingsView(w int) string {
	s := m.sess.Settings()
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", styleHeader.Render("Default OptiScaler version:"), s.DefaultVersion)
	fmt.Fprintf(&b, "%s %s\n", styleHeader.Render("Launch template:"), trunc(s.LaunchTemplate, w-18))
	b.WriteString("\n" + styleHeader.Render("Scan directories") + "\n")
	if len(s.ExtraDirs) == 0 {
		b.WriteString(styleMuted.Render("  none yet — press a to add one") + "\n")
	}
	for i, d := range s.ExtraDirs {
		line := "  " + trunc(d, w-4)
		if i == m.dirCursor {
			line = styleSelected.Render(lipgloss.NewStyle().Width(w).Render("> " + trunc(d, w-4)))
		}
		b.WriteString(line + "\n")
	}
	if m.confirmRmDir != "" {
		b.WriteString("\n" + styleWarn.Render(fmt.Sprintf("remove %s? [y/n]", m.confirmRmDir)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// helpView renders the key reference screen.
func helpView() string {
	return styleHeader.Render("Keyboard reference") + "\n\n" + strings.Join([]string{
		"Global    1 games · 2 settings · 3 help · q / ctrl+c quit",
		"Games     j/k move · enter detail · i install/uninstall · l launch · c cancel",
		"          / filter · R rescan · s sort",
		"Detail    i install · l launch · c cancel · r rollback · o open INI · esc back",
		"Settings  e edit version · t edit template · a add dir · d remove dir",
		"          x clear bundle cache",
		"Confirm   y proceed · n cancel",
	}, "\n")
}
