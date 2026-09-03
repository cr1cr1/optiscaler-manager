package ui

import (
	"path/filepath"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
)

// RefreshInstallState re-probes one row's OptiScaler install state from
// disk and updates the cached row when it drifted. Selection calls it so a
// hook installed, removed, or renamed by hand while the manager sat on
// scan-cached state renders correctly without a rescan. The probe is a
// handful of stats plus one bounded PE identity parse — cheap enough to
// run synchronously on a click.
func (s *Session) RefreshInstallState(dir string) {
	row := s.findRow(dir)
	if row == nil {
		return
	}
	next := *row
	probeRowInstallState(&next)
	if next.Status == row.Status && next.Actionable == row.Actionable &&
		next.Disabled == row.Disabled && next.InjectionDir == row.InjectionDir &&
		next.OptiScalerVersion == row.OptiScalerVersion {
		return
	}
	s.mu.Lock()
	for i := range s.st.Rows {
		if s.st.Rows[i].InstallDir == dir {
			s.st.Rows[i] = next
			break
		}
	}
	s.mu.Unlock()
	s.persistCache()
}

// probeRowInstallState refreshes the on-disk install fields of r in place.
// Manifest semantics mirror the scan's probe: a committed row keeps its
// manifest status (the repair surface owns that), and only the on-disk
// toggle can drift; an external row is whatever the disk says right now,
// including gone.
func probeRowInstallState(r *GameRow) {
	// Interrupted and rolled-back installs belong to the repair surface;
	// partial files on disk must not flip them to external.
	if r.Status != "" && r.Status != domain.StatusCommitted && r.Status != domain.StatusExternal {
		return
	}
	inj := r.InjectionDir
	if inj == "" {
		d, err := discovery.ResolveInstallDir(r.InstallDir)
		if err != nil {
			return
		}
		inj = filepath.Clean(d)
	}
	disabled := pever.DisabledHook(inj) != ""
	if r.Status == domain.StatusCommitted {
		r.InjectionDir = inj
		r.Disabled = disabled
		return
	}
	if r.Status == domain.StatusExternal {
		r.InjectionDir = inj
		if disabled {
			r.Disabled = true
			return
		}
		r.Disabled = false
		if found, version := pever.DetectOptiScaler(inj); found {
			r.OptiScalerVersion = version // "" when the evidence chain runs dry
		} else {
			// The hook vanished by hand: no install anymore.
			r.Status = ""
			r.OptiScalerVersion = ""
		}
		return
	}
	// No install on record: maybe the user dropped OptiScaler in by hand.
	if found, version := pever.DetectOptiScaler(inj); found {
		r.InjectionDir = inj
		r.Status = domain.StatusExternal
		r.Actionable = false
		r.OptiScalerVersion = version
		return
	}
	if pever.DisabledHookVerified(inj) != "" {
		r.InjectionDir = inj
		r.Status = domain.StatusExternal
		r.Actionable = false
		r.Disabled = true
	}
}
