package ui

import (
	"sort"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/version"
)

// Versions returns the per-game list of selectable OptiScaler versions,
// newest first: the union of the row's currently installed version, the
// versions with a usable bundle in the cache, and the configured default
// (preference) version. It feeds the GUI's per-game version dropdown and
// the TUI's version-cycling key.
//
// OFFLINE RULE: Versions never touches the network or the resolver. A
// "latest" preference contributes the already-memoized resolution
// (resolvedDefault) — the concrete tag — and contributes NOTHING when the
// memo is empty (never resolved, resolution failed, or offline); the
// literal string "latest" is never offered because it is not installable.
// Resolution happens at scan/warm-boot time, not at render time. A pinned
// concrete preference needs no resolution and is contributed verbatim,
// cached or not, online or not.
//
// Composition notes for callers:
//   - Dedupe is EXACT-STRING only: installed evidence may be a bare
//     PE-probe version ("0.7.9") while cached and resolved tags are
//     v-prefixed ("v0.9.4"); both forms may legitimately coexist in the
//     list, each verbatim. Sorting uses version.Compare, which already
//     equates the two forms for ORDER.
//   - An unknown gameDir (no row) still returns cached ∪ preference —
//     the same data a row-less dropdown would render — with no installed
//     entry.
//   - When every source is empty the result is an empty (non-nil) slice,
//     so frontends can range over it without a nil check.
func (s *Session) Versions(gameDir string) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(v string) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}

	if row := s.findRow(gameDir); row != nil {
		add(row.OptiScalerVersion)
	}
	for _, v := range app.CachedVersions(s.deps.CacheDir) {
		add(v)
	}
	switch pref := s.Settings().DefaultVersion; pref {
	case "latest":
		add(s.resolvedDefault()) // concrete tag or "" — never "latest"
	case "":
		// No preference configured: nothing to contribute.
	default:
		add(pref) // pinned concrete tag: verbatim, no resolution needed
	}

	sort.Slice(out, func(i, j int) bool {
		return version.Compare(out[i], out[j]) > 0
	})
	return out
}
