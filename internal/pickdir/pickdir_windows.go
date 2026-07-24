//go:build windows

package pickdir

import (
	"context"
	"os/exec"
	"strings"
)

// psScript drives the .NET Windows Forms FolderBrowserDialog via Windows
// PowerShell. PowerShell is always present on Windows 7+ at
// %SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe (on PATH by
// default). The dialog requires a Single-Threaded Apartment, so -STA is
// passed; -NoProfile avoids loading the user's $PROFILE (faster, no
// side-effects); -NonInteractive keeps PS from blocking on prompt inputs.
const psScript = `Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = 'Select game directory'
$d.ShowNewFolderButton = $true
$d.RootFolder = [System.Environment+SpecialFolder]::MyComputer
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
    Write-Output $d.SelectedPath
}`

// Pick opens the OS directory dialog and returns the chosen path.
// Cancelled dialogs return ("", nil).
func Pick(ctx context.Context) (string, error) {
	if _, err := exec.LookPath("powershell"); err != nil {
		return "", ErrUnavailable
	}
	out, err := exec.CommandContext(ctx,
		"powershell", "-NoProfile", "-STA", "-NonInteractive", "-Command", psScript,
	).Output()
	if err != nil {
		// User-cancelled dialog returns a non-zero exit; surface as ("", nil)
		// so callers don't toast "cancelled" as an error.
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
