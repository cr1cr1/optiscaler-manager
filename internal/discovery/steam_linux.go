//go:build linux

package discovery

import (
	"os"
	"path/filepath"
)

// SteamRoots returns existing Steam installation roots on Linux: native,
// Flatpak, and Snap locations, deduplicated through symlinks.
func SteamRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	candidates := []string{
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam"),
		filepath.Join(home, "snap", "steam", "common", ".steam", "steam"),
	}
	var roots []string
	seen := map[string]bool{}
	for _, c := range candidates {
		st, err := os.Stat(c)
		if err != nil || !st.IsDir() {
			continue
		}
		key := c
		if resolved, err := filepath.EvalSymlinks(c); err == nil {
			key = resolved
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		roots = append(roots, c)
	}
	return roots
}
