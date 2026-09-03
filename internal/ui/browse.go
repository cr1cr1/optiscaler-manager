package ui

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/cr1cr1/optiscaler-manager/internal/termopen"
)

// openExternal opens path with the platform's default handler. On Linux
// that is a terminal editor ($EDITOR → micro → nano → vi) running inside a
// terminal emulator, spawned detached; darwin and windows keep the OS
// file handler.
func openExternal(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	default: // linux and the rest
		return termopen.New("", nil, nil, nil).Open(path)
	}
}

// VisibleRows returns the rows after the current query filter and sort.
func (s *Session) VisibleRows() []GameRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := filterRows(append([]GameRow(nil), s.st.Rows...), s.st.Query)
	if s.st.Sort == SortName {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Title < rows[j].Title })
	}
	return rows
}

// SetQuery updates the search filter (cheap, synchronous).
func (s *Session) SetQuery(q string) {
	s.mu.Lock()
	s.st.Query = q
	s.mu.Unlock()
}

// SetSort selects the row ordering VisibleRows applies; out-of-range modes
// reset to SortDefault. Cheap and synchronous like SetQuery.
func (s *Session) SetSort(mode SortMode) {
	if mode != SortName {
		mode = SortDefault
	}
	s.mu.Lock()
	s.st.Sort = mode
	s.mu.Unlock()
}

// ToggleView flips between grid and list presentation.
func (s *Session) ToggleView() {
	s.mu.Lock()
	if s.st.Mode == ViewGrid {
		s.st.Mode = ViewList
	} else {
		s.st.Mode = ViewGrid
	}
	s.mu.Unlock()
}

// Select marks a game as the dashboard target ("" closes it). Selecting a
// game re-probes its install state from disk, so hooks installed, removed,
// or disabled/enabled by hand since the last scan render correctly.
func (s *Session) Select(dir string) {
	if dir != "" {
		s.RefreshInstallState(dir)
	}
	s.mu.Lock()
	s.st.Selected = dir
	s.mu.Unlock()
}

// INIPath returns the game's OptiScaler.ini path, or "" when the game has
// no OptiScaler install (managed or external). Pure resolver with no side
// effects — the GUI's external opener and the TUI's in-process editor both
// build on it.
func (s *Session) INIPath(gameDir string) string {
	row := s.findRow(gameDir)
	if row == nil || row.InjectionDir == "" {
		return ""
	}
	return filepath.Join(row.InjectionDir, "OptiScaler.ini")
}

// OpenINI opens the game's OptiScaler.ini in the system editor (GUI path:
// a terminal editor inside a terminal emulator on Linux via termopen).
func (s *Session) OpenINI(gameDir string) {
	path := s.INIPath(gameDir)
	if path == "" {
		s.toast("no OptiScaler.ini (not installed?)", true)
		return
	}
	if err := s.openExternal(path); err != nil {
		s.toast("cannot open editor: "+err.Error(), true)
	}
}

// Toast surfaces a short message in the active frontend; the TUI uses it
// to report outcomes of work it drives itself (the in-process editor).
func (s *Session) Toast(text string, warn bool) { s.toast(text, warn) }
