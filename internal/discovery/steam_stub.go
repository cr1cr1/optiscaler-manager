//go:build !linux && !windows && !darwin

package discovery

// SteamRoots returns nil: no Steam probe exists for this platform.
func SteamRoots() []string { return nil }
