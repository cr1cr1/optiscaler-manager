package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/umu"
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

// SetUmuEnabled toggles routing manual / Heroic Epic/GOG Windows binaries through
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

// UmuPrefixes returns the detected wine prefixes the prefix pickers
// offer, from the injected deps provider or the production disk scan.
func (s *Session) UmuPrefixes() []umu.Prefix {
	s.mu.Lock()
	fn := s.deps.UmuPrefixes
	s.mu.Unlock()
	if fn != nil {
		return fn()
	}
	return umu.FindPrefixes(context.Background())
}

// SetUmuPrefixPath sets the global default wine prefix umu-run launches
// into. An empty value means Auto: per-game pins and the detection
// chain decide. Persisted.
func (s *Session) SetUmuPrefixPath(path string) {
	msg := "umu prefix: auto"
	if path != "" {
		msg = "umu prefix: " + path
	}
	s.updateSettings(func(st *settings.Settings) bool {
		st.UmuPrefixPath = path
		return true
	}, msg, "")
}

// SetUmuPrefixOverride pins the wine prefix for one game (keyed by
// canonical install dir, the TitleOverrides precedent). A per-game pin
// beats the global UmuPrefixPath default and the auto chain; an empty
// prefix clears the pin. Persisted.
func (s *Session) SetUmuPrefixOverride(gameDir, prefix string) {
	dir := canonicalDir(gameDir)
	if s.findRow(dir) == nil {
		s.toast("unknown game: "+gameDir, true)
		return
	}
	s.updateSettings(func(st *settings.Settings) bool {
		if prefix == "" {
			delete(st.PrefixOverrides, dir)
		} else {
			if st.PrefixOverrides == nil {
				st.PrefixOverrides = map[string]string{}
			}
			st.PrefixOverrides[dir] = prefix
		}
		return true
	}, "", "")
	msg := "umu prefix cleared: " + s.findRow(dir).Title
	if prefix != "" {
		msg = "umu prefix: " + prefix
	}
	s.toast(msg, false)
}

// UmuPrefixPin returns the per-game pinned prefix for dir ("" when the
// game has no override — the per-game picker's current value).
func (s *Session) UmuPrefixPin(gameDir string) string {
	return s.Settings().PrefixOverrides[canonicalDir(gameDir)]
}

// UmuPrefixFor returns the effective pinned prefix for a game: the
// per-game override when set, else the global UmuPrefixPath default,
// else "" (the launch hook runs the auto chain). The launch hook reads
// this at launch time so pins changed mid-session apply immediately.
func (s *Session) UmuPrefixFor(gameDir string) string {
	st := s.Settings()
	if p := st.PrefixOverrides[canonicalDir(gameDir)]; p != "" {
		return p
	}
	return st.UmuPrefixPath
}

// SetActiveFork selects the OptiScaler distribution source installs and
// switches download from (persisted). The GitHub client is rebuilt
// through the NewGH factory, and the fork-scoped memos (startup latest
// tag, resolved default version) are cleared so nothing resolved from the
// previous distribution leaks into the new one. An unknown slug is
// refused with an error and a warn toast.
func (s *Session) SetActiveFork(slug string) error {
	s.mu.Lock()
	var fork settings.Fork
	found := false
	for _, f := range s.deps.Settings.Forks {
		if f.Slug == slug {
			fork, found = f, true
			break
		}
	}
	if !found {
		s.mu.Unlock()
		s.toast("unknown OptiScaler fork: "+slug, true)
		return fmt.Errorf("ui: unknown fork %q", slug)
	}
	if s.deps.Settings.ActiveFork == slug {
		s.mu.Unlock()
		return nil
	}
	s.deps.Settings.ActiveFork = slug
	snap := s.deps.Settings
	s.swapGHLocked(fork)
	s.mu.Unlock()

	return s.persistSettings(snap, "OptiScaler source: "+slug)
}

// AddFork appends a validated distribution source (persisted); validation
// failures and duplicates surface as an error plus a warn toast.
func (s *Session) AddFork(f settings.Fork) error {
	s.mu.Lock()
	err := s.deps.Settings.AddFork(f)
	snap := s.deps.Settings
	s.mu.Unlock()
	if err != nil {
		s.toast(err.Error(), true)
		return err
	}
	return s.persistSettings(snap, "fork added: "+f.Slug)
}

// RemoveFork deletes a distribution source (persisted). Removing the
// active fork resets the selection to upstream and swaps the client back;
// the upstream entry itself is refused.
func (s *Session) RemoveFork(slug string) error {
	s.mu.Lock()
	wasActive := s.deps.Settings.ActiveFork == slug
	err := s.deps.Settings.RemoveFork(slug)
	snap := s.deps.Settings
	if err == nil && wasActive {
		s.swapGHLocked(snap.Active())
	}
	s.mu.Unlock()
	if err != nil {
		s.toast(err.Error(), true)
		return err
	}
	return s.persistSettings(snap, "fork removed: "+slug)
}

// persistSettings saves snap and toasts the outcome: the save error (both
// toasted and returned) or okToast on success.
func (s *Session) persistSettings(snap settings.Settings, okToast string) error {
	if err := settings.Save(s.deps.SettingsRoot, snap); err != nil {
		s.toast("settings not saved: "+err.Error(), true)
		return err
	}
	s.toast(okToast, false)
	return nil
}

// swapGHLocked rebuilds the GitHub client for fork through the NewGH
// factory and clears the fork-scoped memos. Callers hold s.mu. A nil
// factory keeps the injected client (tests that never switch forks).
func (s *Session) swapGHLocked(fork settings.Fork) {
	if s.deps.NewGH != nil {
		s.deps.GH = s.deps.NewGH(fork)
	}
	s.latestTag = ""
	s.resolvedDefaultKey = ""
	s.resolvedDefaultVersion = ""
	s.resolvedDefaultFresh = false
	s.resolvedDefaultAt = time.Time{}
}

// ClearBundleCache deletes all cached OptiScaler bundles (every fork
// namespace). The deletion runs in the background (large caches can take
// a while); a toast reports the outcome.
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
