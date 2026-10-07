//go:build linux

package pickdir

import (
	"context"
	"os/exec"
	"strings"
)

// linuxFilePickers are the Linux image-file dialogs tried in order
// (zenity, then kdialog), each with an image filter preselected.
var linuxFilePickers = [][]string{
	{"zenity", "--file-selection", "--title=Select poster image",
		"--file-filter=Images | *.png *.jpg *.jpeg *.webp *.gif *.bmp"},
	{"kdialog", "--getopenfilename", ".", "Images (*.png *.jpg *.jpeg *.webp *.gif *.bmp)"},
}

// PickFile opens the OS file dialog filtered to images and returns the
// chosen path. Cancelled dialogs return ("", nil).
func PickFile(ctx context.Context) (string, error) {
	for _, cmd := range linuxFilePickers {
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
