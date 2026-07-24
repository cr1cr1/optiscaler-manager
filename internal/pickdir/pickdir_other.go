//go:build !linux && !windows && !darwin

package pickdir

import "context"

// Pick returns ErrUnavailable on platforms without a wired-up native dialog.
// Add a per-platform file (pickdir_<GOOS>.go) to enable the picker.
func Pick(context.Context) (string, error) {
	return "", ErrUnavailable
}
