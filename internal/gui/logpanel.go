package gui

import (
	"strings"
	"time"

	. "go.hasen.dev/shirei"
	. "go.hasen.dev/shirei/widgets"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// Issue 052: the status bar stays one line tall; a Log button on its left
// slides up a readonly, selectable panel showing the same console output
// (the session's status history, including umu stderr tails).

// logAnimDur is the slide-up/down tween duration.
const logAnimDur = 160 * time.Millisecond

// logRingMaxLines mirrors the session ring's capacity (512) with slack so
// an eviction rebuild never strands lines.
const logRingMaxLines = 576

// toggleLogPanel opens/closes the slide-up log panel.
func (m *model) toggleLogPanel() {
	m.logOpen = !m.logOpen
	m.logAnimStart = time.Now()
	if m.logOpen {
		m.logSearchFocusPending = true
	}
}

// logAnimating reports whether the panel tween is still running.
func (m *model) logAnimating() bool {
	return time.Since(m.logAnimStart) < logAnimDur
}

// logT is the eased open fraction: 0 closed, 1 open (ease-out cubic).
func (m *model) logT() float32 {
	d := float32(time.Since(m.logAnimStart)) / float32(logAnimDur)
	if d > 1 {
		d = 1
	}
	if !m.logOpen {
		d = 1 - d
	}
	return 1 - (1-d)*(1-d)*(1-d)
}

// logPanelHeight is the fully-open panel height for a window height.
func logPanelHeight(winH float32) float32 {
	h := winH * 0.35
	if h < 160 {
		return 160
	}
	if h > 320 {
		return 320
	}
	return h
}

// syncLogEntries mirrors session log entries into the LogView ring,
// appending only unseen entries. Game-scoped entries render as
// "Title › line". When the session
// ring evicted (the mirror can no longer line up: fewer entries, or the
// last synced line drifted), the ring rebuilds from the full source.
// (Issues 052, 057.)
func (m *model) syncLogEntries(entries []ui.LogEntry) {
	if m.logRing == nil {
		m.logRing = NewTextRingSize(1<<20, logRingMaxLines)
	}
	if m.logSynced > len(entries) ||
		(m.logSynced > 0 && entries[m.logSynced-1].Line != m.logSyncedLast) {
		m.logRing = NewTextRingSize(1<<20, logRingMaxLines)
		m.logSynced = 0
		m.logSyncedLast = ""
	}
	for ; m.logSynced < len(entries); m.logSynced++ {
		e := entries[m.logSynced]
		if e.Game == "" {
			m.logRing.AppendLine(e.Line)
			continue
		}
		m.logRing.AppendLine(e.Game + " › " + e.Line)
	}
	if m.logSynced > 0 {
		m.logSyncedLast = entries[m.logSynced-1].Line
	}
}

// fitStatusLine (issue 072) truncates the one-line status to maxW pixels
// with an ellipsis so the status bar's intrinsic width can never exceed the
// window: an unbounded long status inflates the main column's resolved width
// (shirei's distribution grows but never shrinks children), which poisons
// the width feedback every virtual list — the log view included — shapes
// against. maxW <= 0 (width unknown, first frame) leaves the text alone.
func fitStatusLine(s string, maxW float32) string {
	if maxW <= 0 || s == "" {
		return s
	}
	st := statusTextStyle()
	if len(ShapeTextMax(s, st, maxW).Lines) <= 1 {
		return s
	}
	runes := []rune(s)
	// Longest prefix that fits one line with an ellipsis appended; the
	// predicate is monotone, so binary search. ShapeTextMax is cached —
	// the ~log2(n) probes of the same paragraph are cheap.
	lo, hi := 0, len(runes)-1 // full string is known not to fit
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if len(ShapeTextMax(string(runes[:mid])+"…", st, maxW).Lines) <= 1 {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo]) + "…"
}

// statusTextStyle mirrors muted() (status bar text) for width measurement.
func statusTextStyle() TextStyleAttrs {
	return TextStyleAttrs{TextColor: txtMuted, FontSize: 12}
}

// oneLineStatus collapses the status text to its first line for the
// single-line status bar; the full text lives in the log panel.
func oneLineStatus(s string) string {
	line, rest, cut := strings.Cut(s, "\n")
	if cut && strings.TrimSpace(rest) != "" {
		line += " …"
	}
	return line
}

// logListKey addresses the log view's virtual list for scroll commands
// (issue 054).
const logListKey = "log-list"

// logSearchFind returns the first ring line index at or past start
// (wrapping) that contains query case-insensitively; -1 when the query
// is empty or nothing matches (issue 054).
func (m *model) logSearchFind(query string, start int) int {
	q := strings.ToLower(query)
	if q == "" || m.logRing == nil {
		return -1
	}
	n := m.logRing.Len()
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		if strings.Contains(strings.ToLower(m.logRing.Line(idx)), q) {
			return idx
		}
	}
	return -1
}

// logSearchDebounce is the typing pause before the field buffer becomes
// the applied highlight query (issue 055).
const logSearchDebounce = 300 * time.Millisecond

// syncLogSearchQuery promotes the search buffer to the applied query once
// 300ms pass without further typing; while armed it keeps the pulse
// ticker awake so the promotion frame lands on time (issue 040
// mechanism). Explicit search flushes immediately via logSearchNext.
func (m *model) syncLogSearchQuery() {
	if m.logSearchBuf != m.logSearchPrevBuf {
		m.logSearchPrevBuf = m.logSearchBuf
		m.logSearchDirtyAt = time.Now()
		m.logSearchPending = true
	}
	if !m.logSearchPending {
		return
	}
	if time.Since(m.logSearchDirtyAt) >= logSearchDebounce {
		m.logSearchQuery = m.logSearchBuf
		m.logSearchPending = false
		return
	}
	pulseWanted.Store(true)
}

// logSearchNext jumps the log view to the next line matching the search
// buffer: an unchanged query advances past the current hit and wraps
// (issue 054). The explicit submit flushes the typing debounce, so the
// highlight applies without waiting (issue 055).
func (m *model) logSearchNext() {
	m.logSearchQuery = m.logSearchBuf
	m.logSearchPrevBuf = m.logSearchBuf
	m.logSearchPending = false
	q := m.logSearchQuery
	start := 0
	if q == m.logSearchLast && m.logSearchHitLine >= 0 {
		start = m.logSearchHitLine + 1
	}
	hit := m.logSearchFind(q, start)
	m.logSearchHitLine = hit
	m.logSearchLast = q
	if hit >= 0 {
		VirtualListScrollIntoView(logListKey, m.logRing.LineID(hit))
	}
}

// logTextStyle is the log view's text style: the default monospace stack
// (issue 054).
func logTextStyle() TextStyleAttrs {
	attrs := TextStyleAttrs{TextColor: txtMain, FontSize: 12}
	attrs.SetFontFamilies(Monospace...)
	return attrs
}

// logPanel renders the slide-up log view between the content and the
// status bar. It stays mounted while open and while the close tween runs;
// the tween drives FixHeight directly (NoAnimate blocks shirei's own
// layout easing, which would double-animate the height).
func (m *model) logPanel() {
	m.logPanelRect = Rect{}
	m.logPanelResolvedW = 0
	t := m.logT()
	if t <= 0 {
		return
	}
	if m.logAnimating() {
		RequestNextFrame()
	}
	h := logPanelHeight(GetHost().WindowSize[1]) * t
	Container(Attrs(Expand, FixHeight(h), NoAnimate, Clip, BackgroundVec(bgPanel), BorderWidth(1), BorderColorVec(border)), func() {
		m.logPanelRect = GetScreenRectOf(CurrentId())
		m.logPanelResolvedW = GetResolvedSize()[0] // issue 072 test seam: must equal the on-screen width
		Container(Attrs(Row, CrossMid, Pad2(sp4, sp12), Gap(sp12)), func() {
			Label("Log", TextColorVec(txtMain), FontSize(12), FontWeight(WeightBold))
			Filler(1)
			if m.logSearchState == nil {
				m.logSearchState = &editState{anchor: -1, blink: time.Now(), phase: true, onEnter: m.logSearchNext}
			}
			themedInputState(&m.logSearchBuf, "search log", NoIcon, m.logSearchState,
				MinSize(160, fieldH), MaxSizeVec(Vec2{280, fieldH}))
			if m.logSearchFocusPending {
				m.logSearchFocusPending = false
				FocusImmediateOn(GetLastId())
			}
			m.syncLogSearchQuery()
			Container(Attrs(Row), func() {
				m.logSearchBtnRect = GetScreenRectOf(CurrentId())
				if focusableButton(TypZoom, "") {
					m.logSearchNext()
				}
			})
			// Close stays the right-most control (issue 054).
			Container(Attrs(Row), func() {
				m.logCloseRect = GetScreenRectOf(CurrentId())
				if focusableButton(NoIcon, "Close") {
					m.toggleLogPanel()
				}
			})
		})
		Container(Attrs(Grow(1), Expand, Pad2(0, sp8), Clip), func() {
			if m.logRing == nil || m.logRing.Len() == 0 {
				muted("No log lines yet")
				return
			}
			// LogView is readonly by construction: drag-selects across
			// lines, copies with Ctrl/Cmd+C, pins to the newest line.
			// Search still scrolls to the next match (logSearchNext);
			// the hit highlight and colored game-title prefixes were
			// vendor patches (v0.22/v0.23), dropped in the shirei v0.9.1
			// upgrade — see docs/vendor-patches.md.
			LogViewExt(m.logRing, logTextStyle(), logListKey, &m.logProbe)
		})
	})
}
