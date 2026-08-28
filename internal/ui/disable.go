package ui

import (
	"os"
	"path/filepath"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
)

// ToggleDisabled flips the game's OptiScaler install between enabled and
// disabled by renaming its injection hook: dxgi.dll ↔ dxgi.dll.disabled
// (or whichever known hook the install uses, see pever). A disabled
// install stays on disk and keeps its status — the game simply does not
// load the hook until the rename is reverted. The rename is atomic and
// instant, so this runs synchronously and settles with a toast.
func (s *Session) ToggleDisabled(gameDir string) {
	row := s.findRow(gameDir)
	if row == nil {
		s.toast("unknown game: "+gameDir, true)
		return
	}
	if row.InjectionDir == "" || (row.Status != domain.StatusCommitted && row.Status != domain.StatusExternal) {
		s.toast("OptiScaler is not installed for "+gameTitle(row, gameDir), true)
		return
	}
	dir := row.InjectionDir
	if name := pever.DisabledHook(dir); name != "" {
		if err := os.Rename(filepath.Join(dir, name+pever.DisabledSuffix), filepath.Join(dir, name)); err != nil {
			s.toast("enable failed: "+err.Error(), true)
			return
		}
		s.setRowDisabled(gameDir, false)
		s.toast("OptiScaler enabled: "+row.Title, false)
		return
	}
	name := pever.ActiveHook(dir)
	if name == "" {
		s.toast("no OptiScaler hook found in "+dir, true)
		return
	}
	if err := os.Rename(filepath.Join(dir, name), filepath.Join(dir, name+pever.DisabledSuffix)); err != nil {
		s.toast("disable failed: "+err.Error(), true)
		return
	}
	s.setRowDisabled(gameDir, true)
	s.toast("OptiScaler disabled: "+row.Title, false)
}

// setRowDisabled updates a row's Disabled flag and rewrites the games
// cache, mirroring setRowStatus.
func (s *Session) setRowDisabled(dir string, disabled bool) {
	s.mu.Lock()
	changed := false
	for i := range s.st.Rows {
		if s.st.Rows[i].InstallDir == dir {
			s.st.Rows[i].Disabled = disabled
			changed = true
			break
		}
	}
	s.mu.Unlock()
	if changed {
		s.persistCache()
	}
}
