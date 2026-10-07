//go:build darwin

package pickdir

import (
	"context"
	"os/exec"
	"strings"
)

// PickFile opens the OS file dialog filtered to images and returns the
// chosen path. Cancelled dialogs return ("", nil). osascript is always
// present on macOS.
func PickFile(ctx context.Context) (string, error) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return "", ErrUnavailable
	}
	out, err := exec.CommandContext(ctx,
		"osascript", "-e",
		`POSIX path of (choose file of type {"public.image"} with prompt "Select poster image")`,
	).Output()
	if err != nil {
		// osascript exits non-zero when the user clicks Cancel; treat that
		// as ("", nil) so the caller doesn't toast "cancelled" as an error.
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
