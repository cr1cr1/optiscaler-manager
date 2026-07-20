package pever

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// injectCandidates are the DLL names OptiScaler uses as an injection shim
// (renamed or as-is), checked in this order; the first match wins.
var injectCandidates = []string{
	"dxgi.dll",
	"OptiScaler.dll",
	"winmm.dll",
	"dbghelp.dll",
	"version.dll",
	"wininet.dll",
	"winhttp.dll",
	"d3d12.dll",
}

// identityKeys are the StringFileInfo keys inspected for an OptiScaler
// identity marker.
var identityKeys = []string{"ProductName", "CompanyName", "OriginalFilename"}

// DetectOptiScaler reports whether dir contains an OptiScaler injection
// DLL. A candidate matches when any of its ProductName, CompanyName, or
// OriginalFilename StringFileInfo values contains "optiscaler"
// (case-insensitive). Missing, unreadable, oversized, or non-PE candidates
// are skipped silently; a non-matching candidate (e.g. a DXVK dxgi.dll)
// does not stop the scan.
//
// When found, the version comes from the evidence chain: manifest.json →
// OptiScaler.log banner → the matched candidate's PE version resource →
// "" (installed but version unknown).
func DetectOptiScaler(dir string) (found bool, version string) {
	for _, name := range injectCandidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		res, ok := candidateResources(path)
		if !ok {
			continue
		}
		if !hasOptiScalerIdentity(res) {
			continue
		}
		return true, detectVersion(dir, res)
	}
	return false, ""
}

// candidateResources reads path and returns its version-resource bytes, or
// false when the file is unreadable, oversized, or not a PE with a
// resource section.
func candidateResources(path string) ([]byte, bool) {
	data, err := ReadBounded(path, maxPEFileSize)
	if err != nil {
		log.Debug().Err(err).Str("path", path).Msg("detect: candidate unreadable")
		return nil, false
	}
	res, err := resourceBytes(data)
	if err != nil {
		log.Debug().Err(err).Str("path", path).Msg("detect: candidate not a PE with resources")
		return nil, false
	}
	if len(res) > maxVersionScan {
		res = res[:maxVersionScan]
	}
	return res, true
}

// hasOptiScalerIdentity reports whether any identity key's value mentions
// OptiScaler. One resource buffer serves all three lookups.
func hasOptiScalerIdentity(res []byte) bool {
	for _, key := range identityKeys {
		if strings.Contains(strings.ToLower(scanStringFileInfoKey(res, key)), "optiscaler") {
			return true
		}
	}
	return false
}

// detectVersion applies the manifest → log → PE evidence chain, reusing
// the matched candidate's resource bytes for the PE fallback.
func detectVersion(dir string, res []byte) string {
	if v := manifestVersion(filepath.Join(dir, "manifest.json")); v != "" {
		return v
	}
	if v := logVersion(filepath.Join(dir, "OptiScaler.log")); v != "" {
		return v
	}
	fixed := scanFixedFileInfo(res)
	product := scanStringFileInfoKey(res, "ProductVersion")
	return normalize(fixed, product)
}
