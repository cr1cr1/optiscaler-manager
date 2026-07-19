//go:build windows

package discovery

import "os"

// epicManifestDirs returns the Epic Games Launcher manifest directory on
// Windows: %ProgramData%\Epic\EpicGamesLauncher\Data\Manifests.
func epicManifestDirs() []string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		return nil
	}
	return []string{programData + `\Epic\EpicGamesLauncher\Data\Manifests`}
}
