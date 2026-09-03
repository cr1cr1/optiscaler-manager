package pever

import (
	"os"
	"path/filepath"
	"strings"
)

// DisabledSuffix is the rename suffix the disable toggle applies to the
// injection hook: dxgi.dll ↔ dxgi.dll.disabled. A renamed hook is not
// loaded by the game, so OptiScaler is off until the rename is reverted.
const DisabledSuffix = ".disabled"

// hookCandidates are the OptiScaler injection hook names the disable
// toggle knows how to rename, checked in this order. It mirrors
// injectCandidates (OptiScaler.dll aside: the bundle's own name is never
// the in-game hook) plus OptiScaler.asi, the ASI-loader variant.
var hookCandidates = []string{
	"dxgi.dll",
	"winmm.dll",
	"version.dll",
	"dbghelp.dll",
	"d3d12.dll",
	"wininet.dll",
	"winhttp.dll",
	"OptiScaler.asi",
}

// ActiveHook returns the name of dir's OptiScaler injection hook, "" when
// no candidate is present. Like DetectOptiScaler, a candidate must carry
// an OptiScaler identity marker: the disable toggle renames this file, so
// a lookalike hook (DXVK's dxgi.dll) must never be the answer.
func ActiveHook(dir string) string {
	for _, name := range hookCandidates {
		if hookIdentified(filepath.Join(dir, name)) {
			return name
		}
	}
	return ""
}

// DisabledHook returns the hook disabled in dir: hook is the known
// injection name the game would load (dxgi.dll), file the actual
// renamed-away file on disk (dxgi.dll.disabled, dxgi.dll.1, dxgi.dll.bak).
// Both "" when none. Presence is the whole rule: a known hook name
// carrying ANY suffix means OptiScaler is parked, even when the file
// itself cannot be identity-verified (hand-renamed, stripped,
// unreadable). The manager's own .disabled suffix wins over backup-style
// suffixes so the toggle round-trips deterministically. Use
// DisabledHookVerified instead when the answer decides whether an
// UNMANAGED game counts as an OptiScaler install at all — a DXVK
// dxgi.dll.1 is not one.
func DisabledHook(dir string) (hook, file string) {
	return disabledHook(dir, false)
}

// DisabledHookVerified is DisabledHook with the OptiScaler identity gate
// DetectOptiScaler applies: the renamed file must parse as a PE carrying
// an OptiScaler marker. First-time external detection of unmanaged
// directories uses it so lookalike mods are never mislabeled; rows whose
// install is already established (manifest, earlier probe) use the
// presence-only DisabledHook.
func DisabledHookVerified(dir string) (hook, file string) {
	return disabledHook(dir, true)
}

// disabledHook scans dir for a renamed-away hook. The manager suffix pass
// runs first across all candidates; the second pass accepts any
// <hook>.<suffix> file in candidate order (dir entries are sorted, so the
// answer is deterministic). verified selects the identity gate.
func disabledHook(dir string, verified bool) (hook, file string) {
	match := func(path string) bool {
		if verified {
			return hookIdentified(path)
		}
		st, err := os.Stat(path)
		return err == nil && !st.IsDir()
	}
	for _, name := range hookCandidates {
		if f := name + DisabledSuffix; match(filepath.Join(dir, f)) {
			return name, f
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", ""
	}
	for _, name := range hookCandidates {
		prefix := name + "."
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
				continue
			}
			if f := e.Name(); f != name+DisabledSuffix && match(filepath.Join(dir, f)) {
				return name, f
			}
		}
	}
	return "", ""
}

// hookIdentified reports whether path is a regular file carrying an
// OptiScaler identity marker. Missing/unreadable/non-PE files are false.
func hookIdentified(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	res, ok := candidateResources(path)
	if !ok {
		return false
	}
	return hasOptiScalerIdentity(res)
}
