//go:build !linux && !windows && !darwin

package pickdir

import "context"

// PickFile returns ErrUnavailable on platforms without a wired-up native
// dialog. Add a per-platform file (pickfile_<GOOS>.go) to enable it.
func PickFile(context.Context) (string, error) {
	return "", ErrUnavailable
}
