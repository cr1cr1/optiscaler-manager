//go:build !windows

package discovery

import "github.com/cr1cr1/optiscaler-manager/internal/domain"

// gogGames returns nil: GOG Galaxy's registry catalogue is Windows-only.
func gogGames() []domain.Game { return nil }
