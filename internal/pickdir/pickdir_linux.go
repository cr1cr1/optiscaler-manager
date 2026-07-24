//go:build linux

package pickdir

import (
	"context"
	"os/exec"
	"strings"
)

// linuxPickers are the Linux directory-dialog commands tried in order. The
// first one on PATH wins; both are common on desktop distros.
var linuxPickers = [][]string{
	{"zenity", "--file-selection", "--directory", "--title=Select game directory"},
	{"kdialog", "--getexistingdirectory", ".", "--title", "Select game directory"},
}

// Pick opens the OS directory dialog and returns the chosen path.
// Cancelled dialogs return ("", nil).
func Pick(ctx context.Context) (string, error) {
	for _, cmd := range linuxPickers {
		if _, err := exec.LookPath(cmd[0]); err != nil {
			continue
		}
		out, err := exec.CommandContext(ctx, cmd[0], cmd[1:]...).Output()
		if err != nil {
			if _, ok := err.(*exec.ExitError); ok {
				return "", nil // user cancelled
			}
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", ErrUnavailable
}
