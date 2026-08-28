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

// DisabledHook returns the hook name disabled in dir — <name>.disabled
// present as a regular file — "" when none. Presence is the whole rule:
// a known hook name carrying the suffix means OptiScaler is disabled,
// even when the file itself cannot be identity-verified (hand-renamed,
// stripped, unreadable). Use DisabledHookVerified instead when the answer
// decides whether an UNMANAGED game counts as an OptiScaler install at
// all — a DXVK dxgi.dll.disabled is not one.
func DisabledHook(dir string) string {
	for _, name := range hookCandidates {
		if st, err := os.Stat(filepath.Join(dir, name+DisabledSuffix)); err == nil && !st.IsDir() {
			return name
		}
	}
	return ""
}

// DisabledHookVerified is DisabledHook with the OptiScaler identity gate
// DetectOptiScaler applies: the .disabled file must parse as a PE
// carrying an OptiScaler marker. First-time external detection of
// unmanaged directories uses it so lookalike mods are never mislabeled;
// rows whose install is already established (manifest, earlier probe)
// use the presence-only DisabledHook.
func DisabledHookVerified(dir string) string {
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
