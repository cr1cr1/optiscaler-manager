//go:build darwin

package discovery

import (
	"os"
	"path/filepath"
)

// epicManifestDirs returns the Epic Games Launcher manifest directory on
// macOS: ~/Library/Application Support/Epic/EpicGamesLauncher/Data/Manifests.
func epicManifestDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{filepath.Join(home, "Library", "Application Support", "Epic", "EpicGamesLauncher", "Data", "Manifests")}
}
