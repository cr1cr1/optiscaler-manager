//go:build !linux

package discovery

import "github.com/cr1cr1/optiscaler-manager/internal/domain"

// heroicGames returns nil: Heroic Games Launcher is a Linux storefront.
func heroicGames() []domain.Game { return nil }
