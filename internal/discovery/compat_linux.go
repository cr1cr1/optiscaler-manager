//go:build linux

package discovery

import (
	"os"
	"path/filepath"
)

// compatPrefix returns the Proton prefix for appID inside libraryPath
// (steamapps/compatdata/<appid>/pfx), or "" when the game has never run
// under Proton. Display-only: the prefix appears only after the first
// Proton launch.
func compatPrefix(libraryPath, appID string) string {
	p := filepath.Join(libraryPath, "steamapps", "compatdata", appID, "pfx")
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p
	}
	return ""
}
