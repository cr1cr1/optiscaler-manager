//go:build !darwin

package discovery

import "github.com/cr1cr1/optiscaler-manager/internal/domain"

// storeApps returns nil: .app bundle scanning is a macOS-only probe.
func storeApps() []domain.Game { return nil }
