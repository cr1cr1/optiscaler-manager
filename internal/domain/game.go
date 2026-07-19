// Package domain defines the core types shared across optiscaler-manager:
// discovered games, upscaler components, resolved release assets, and the
// install manifest that the installer drives and the store persists.
//
// AI agents: read this package first. These types are the vocabulary every
// other module (discovery, gh, installer, store, gui) speaks; change them
// only with a milestone-level decision recorded in docs/.
package domain

// Game is a discovered game installation (M2a discovery produces these).
type Game struct {
	AppID       string
	Name        string
	InstallDir  string // canonical game root
	LibraryPath string
}
