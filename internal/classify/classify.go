// Package classify detects which upscaler technologies a game directory
// contains, by DLL filename. It reports kind + DLL filename only; PE version
// parsing is explicitly out of scope (docs/scope.md).
package classify

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// table maps exact lowercase DLL base names to component kinds.
var table = []struct {
	kind domain.Kind
	name string
}{
	{domain.KindDLSS, "nvngx_dlss.dll"},
	{domain.KindDLSSFG, "nvngx_dlssg.dll"},
	{domain.KindXeSS, "libxess.dll"},
	{domain.KindFSR, "amd_fidelityfx_upscaler_dx12.dll"},
	{domain.KindFSR, "amd_fidelityfx_dx12.dll"},
	{domain.KindFSR, "amd_fidelityfx_vk.dll"},
	{domain.KindFSR, "amd_fidelityfx.dll"},
	{domain.KindFSR, "ffx_fsr2_api_x64.dll"},
	{domain.KindFSR, "ffx_fsr3_api_x64.dll"},
	{domain.KindFSR, "ffx_fsr3upscaler_x64.dll"},
	{domain.KindFSR, "ffx_fsr4upscaler_x64.dll"},
}

// match returns the component kind for a DLL base name, case-insensitively.
// Future amd_fidelityfx_*/ffx_* names fall through to the FSR prefix rule.
func match(name string) (domain.Kind, bool) {
	l := strings.ToLower(name)
	for _, e := range table {
		if l == e.name {
			return e.kind, true
		}
	}
	if strings.HasPrefix(l, "amd_fidelityfx_") || strings.HasPrefix(l, "ffx_") {
		return domain.KindFSR, true
	}
	return 0, false
}

// Dir walks dir recursively (skipping noisy dirs like .git) and returns the
// detected upscaler components, deduplicated by (Kind, DLL) and sorted.
// DLL is the base filename as found on disk.
func Dir(dir string) []domain.Component {
	seen := map[domain.Component]struct{}{}
	var out []domain.Component
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".dll") {
			return nil
		}
		kind, ok := match(d.Name())
		if !ok {
			return nil
		}
		c := domain.Component{Kind: kind, DLL: d.Name()}
		if _, dup := seen[c]; dup {
			return nil
		}
		seen[c] = struct{}{}
		out = append(out, c)
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].DLL < out[j].DLL
	})
	return out
}
