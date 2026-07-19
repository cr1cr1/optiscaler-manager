// Package pickdir asks the OS for a directory using the available native
// dialog (zenity, then kdialog). go-shirei has no native dialogs, so the
// desktop's own chooser is shelled out to.
package pickdir

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// ErrUnavailable means no supported directory-picker tool was found.
var ErrUnavailable = errors.New("no directory picker available (install zenity or kdialog)")

// Pick opens the OS directory dialog and returns the chosen path.
// Cancelled dialogs return ("", nil).
func Pick(ctx context.Context) (string, error) {
	for _, cmd := range [][]string{
		{"zenity", "--file-selection", "--directory", "--title=Select game directory"},
		{"kdialog", "--getexistingdirectory", ".", "--title", "Select game directory"},
	} {
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
