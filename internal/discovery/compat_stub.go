//go:build !linux

package discovery

// compatPrefix always returns "": Proton prefixes are a Linux Steam concept.
func compatPrefix(libraryPath, appID string) string { return "" }
