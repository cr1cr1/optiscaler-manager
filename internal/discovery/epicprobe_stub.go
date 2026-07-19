//go:build !windows && !darwin

package discovery

// epicManifestDirs returns nil: there is no Epic Games Launcher on this
// platform.
func epicManifestDirs() []string { return nil }
