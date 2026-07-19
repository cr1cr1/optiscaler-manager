//go:build windows

package discovery

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// SteamRoots returns the Steam installation root on Windows, read from
// HKLM\SOFTWARE\Valve\Steam InstallPath. The 64-bit view is tried first,
// then the 32-bit (WOW64) view.
func SteamRoots() []string {
	for _, access := range []uint32{
		registry.QUERY_VALUE,
		registry.QUERY_VALUE | registry.WOW64_32KEY,
	} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`, access)
		if err != nil {
			continue
		}
		path, _, err := k.GetStringValue("InstallPath")
		_ = k.Close()
		if err != nil || path == "" {
			continue
		}
		if st, err := os.Stat(path); err == nil && st.IsDir() {
			return []string{path}
		}
	}
	return nil
}
