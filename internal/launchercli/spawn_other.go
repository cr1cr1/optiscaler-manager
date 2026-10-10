//go:build !linux

package launchercli

import (
	"context"
	"fmt"
	"runtime"
)

// Launch is unreachable off Linux (the router only exists there); the
// stub keeps the package compiling cross-platform.
func Launch(_ context.Context, _ Opts) (int, error) {
	return 0, fmt.Errorf("launchercli: launcher CLIs are a Linux concept, not %s", runtime.GOOS)
}
