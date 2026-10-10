//go:build !linux

package optiscalermanager

import (
	"context"

	"github.com/cr1cr1/optiscaler-manager/internal/ui"
)

// newUmuLauncher returns (nil, nil) on non-Linux platforms: umu-launcher
// is a Linux-only tool. The session transparently falls through to the
// regular Launcher for every game.
func newUmuLauncher(_, _ func(installDir string) string, _ func() bool, _ string, _ func(installDir string) string, _ func(ctx context.Context, row ui.GameRow) (string, string), _ func(installDir string) map[string]string) (ui.UmuLauncherHook, func(dir string) bool) {
	return nil, nil
}
