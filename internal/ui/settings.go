package ui

import (
	"path/filepath"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// Settings returns a snapshot of the current user settings; the ExtraDirs
// slice is deep-copied so callers may iterate it while mutators run.
func (s *Session) Settings() settings.Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.deps.Settings
	out.ExtraDirs = append([]string(nil), out.ExtraDirs...)
	return out
}

// updateSettings runs mutate against the live settings under the lock.
// When mutate reports a change the snapshot is persisted: a save failure
// toasts the error, a success toasts toastOK. An unchanged result skips
// the save and toasts toastUnchanged. An empty toast string stays silent.
func (s *Session) updateSettings(mutate func(*settings.Settings) (changed bool), toastOK, toastUnchanged string) {
	s.mu.Lock()
	changed := mutate(&s.deps.Settings)
	snap := s.deps.Settings
	s.mu.Unlock()
	if !changed {
		if toastUnchanged != "" {
			s.toast(toastUnchanged, false)
		}
		return
	}
	if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
		s.toast("settings not saved: "+err.Error(), true)
		return
	}
	if toastOK != "" {
		s.toast(toastOK, false)
	}
}

// SetDefaultVersion changes the release tag installs resolve to (persisted).
func (s *Session) SetDefaultVersion(v string) {
	if v == "" {
		v = "latest"
	}
	s.updateSettings(func(st *settings.Settings) bool {
		if st.DefaultVersion == v {
			return false
		}
		st.DefaultVersion = v
		return true
	}, "default version: "+v, "default version: "+v)
}

// SetOnlineLookups toggles ProtonDB/Steam game-info enrichment during
// scans (persisted); frontends render it as the online-lookups switch.
func (s *Session) SetOnlineLookups(v bool) {
	msg := "online lookups: off"
	if v {
		msg = "online lookups: on"
	}
	s.updateSettings(func(st *settings.Settings) bool {
		st.OnlineLookups = v
		return true
	}, msg, "")
}

// SetCardSize changes the grid card width preset (persisted). Anything
// outside the known presets falls back to medium.
func (s *Session) SetCardSize(size settings.CardSize) {
	size = size.OrDefault()
	s.updateSettings(func(st *settings.Settings) bool {
		if st.CardSize == size {
			return false
		}
		st.CardSize = size
		return true
	}, "", "")
}

// SetLaunchTemplate changes the command template manual games launch with
// (persisted); an empty value resets to the plain `"{exe}" {args}` default.
func (s *Session) SetLaunchTemplate(tmpl string) {
	if tmpl == "" {
		tmpl = settings.DefaultLaunchTemplate
	}
	s.updateSettings(func(st *settings.Settings) bool {
		st.LaunchTemplate = tmpl
		return true
	}, "launch template: "+tmpl, "")
}

// SetUmuEnabled toggles routing manual-store Windows binaries through
// umu-launcher (Linux only). Persisted atomically; toasts the result.
func (s *Session) SetUmuEnabled(enabled bool) {
	msg := "umu-launcher disabled"
	if enabled {
		msg = "umu-launcher enabled"
	}
	s.updateSettings(func(st *settings.Settings) bool {
		st.UmuEnabled = enabled
		return true
	}, msg, "")
}

// SetUmuProtonPath pins the Proton build umu-run uses. An empty value
// means "let umu resolve its own default (UMU-Latest)". Persisted.
func (s *Session) SetUmuProtonPath(path string) {
	msg := "umu Proton: auto"
	if path != "" {
		msg = "umu Proton: " + path
	}
	s.updateSettings(func(st *settings.Settings) bool {
		st.UmuProtonPath = path
		return true
	}, msg, "")
}

// ClearBundleCache deletes all cached OptiScaler bundles. The deletion runs
// in the background (large caches can take a while); a toast reports the
// outcome.
func (s *Session) ClearBundleCache() {
	dir := filepath.Join(s.deps.CacheDir, "optiscaler")
	go func() {
		if err := s.removeAll(dir); err != nil {
			s.toast("clear cache: "+err.Error(), true)
			return
		}
		s.toast("OptiScaler cache cleared", false)
	}()
}
