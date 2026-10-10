//go:build !linux

package optiscalermanager

import (
	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// newUmuLauncher returns nil on non-Linux platforms: umu-launcher is a
// Linux-only tool. The session transparently falls through to the
// regular Launcher for every game.
func newUmuLauncher(_, _ func(installDir string) string, _ func() bool) ui.UmuLauncherHook {
	return nil
}
