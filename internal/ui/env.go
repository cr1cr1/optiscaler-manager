package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cr1cr1/optiscaler-manager/internal/settings"
)

// Per-game launch env (issue 067): an open dictionary of extra env
// vars (DXVK_*, PROTON_*, WINEDLLOVERRIDES, …) passed through to the
// launched process, the umu design notes' "per-game env map". The
// edit format is whitespace-separated KEY=VALUE pairs.

// ParseEnv parses whitespace-separated KEY=VALUE pairs. A token
// without '=' or with an empty key is an error naming it.
func ParseEnv(s string) (map[string]string, error) {
	out := map[string]string{}
	for _, tok := range strings.Fields(s) {
		k, v, ok := strings.Cut(tok, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid env token %q — want KEY=VALUE", tok)
		}
		out[k] = v
	}
	return out, nil
}

// FormatEnv renders an env map in the canonical (sorted) edit form.
func FormatEnv(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + m[k]
	}
	return strings.Join(parts, " ")
}

// UmuEnvFor returns the per-game launch env pin (a copy —
// mutation-safe); empty when unpinned.
func (s *Session) UmuEnvFor(gameDir string) map[string]string {
	pin := s.Settings().EnvOverrides[canonicalDir(gameDir)]
	out := make(map[string]string, len(pin))
	for k, v := range pin {
		out[k] = v
	}
	return out
}

// SetUmuEnvOverride pins the launch env for one game (keyed by
// canonical install dir, the PrefixOverrides precedent); nil or empty
// clears the pin. Persisted.
func (s *Session) SetUmuEnvOverride(gameDir string, env map[string]string) {
	dir := canonicalDir(gameDir)
	if s.findRow(dir) == nil {
		s.toast("unknown game: "+gameDir, true)
		return
	}
	s.updateSettings(func(st *settings.Settings) bool {
		if len(env) == 0 {
			delete(st.EnvOverrides, dir)
		} else {
			if st.EnvOverrides == nil {
				st.EnvOverrides = map[string]map[string]string{}
			}
			pin := make(map[string]string, len(env))
			for k, v := range env {
				pin[k] = v
			}
			st.EnvOverrides[dir] = pin
		}
		return true
	}, "", "")
	msg := "launch env cleared: " + s.findRow(dir).Title
	if len(env) > 0 {
		msg = "launch env: " + FormatEnv(env)
	}
	s.toast(msg, false)
}
