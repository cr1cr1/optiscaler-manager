package pever

import (
	"os"
	"path/filepath"
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
// an OptiScaler identity marker, so a lookalike hook (DXVK's dxgi.dll)
// is never the answer.
func ActiveHook(dir string) string {
	for _, name := range hookCandidates {
		if hookIdentified(filepath.Join(dir, name)) {
			return name
		}
	}
	return ""
}

// DisabledHook returns the hook name disabled in dir — <name>.disabled
// present and OptiScaler-branded — "" when none. OptiScaler itself is
// considered disabled while any known hook carries the suffix.
func DisabledHook(dir string) string {
	for _, name := range hookCandidates {
		if hookIdentified(filepath.Join(dir, name+DisabledSuffix)) {
			return name
		}
	}
	return ""
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
