//go:build darwin

package discovery

import (
	"os"
	"path/filepath"
)

// SteamRoots returns the Steam installation root on macOS:
// ~/Library/Application Support/Steam.
func SteamRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	root := filepath.Join(home, "Library", "Application Support", "Steam")
	if st, err := os.Stat(root); err == nil && st.IsDir() {
		return []string{root}
	}
	return nil
}
