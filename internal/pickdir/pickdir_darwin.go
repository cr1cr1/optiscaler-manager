//go:build darwin

package pickdir

import (
	"context"
	"os/exec"
	"strings"
)

// Pick opens the OS directory dialog and returns the chosen path.
// Cancelled dialogs return ("", nil). osascript is always present on macOS.
func Pick(ctx context.Context) (string, error) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return "", ErrUnavailable
	}
	out, err := exec.CommandContext(ctx,
		"osascript", "-e",
		`POSIX path of (choose folder with prompt "Select game directory")`,
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
