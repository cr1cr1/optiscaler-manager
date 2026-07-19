//go:build windows

package discovery

import "github.com/cr1cr1/optiscaler-manager/internal/domain"

// gogRegistryBase is the HKLM key GOG Galaxy registers installed games under.
const gogRegistryBase = `SOFTWARE\WOW6432Node\GOG.com\Games`

// gogGames returns installed GOG games discovered from the Windows registry.
func gogGames() []domain.Game {
	return gogGamesFromRegistry(windowsRegistry{}, gogRegistryBase)
}
