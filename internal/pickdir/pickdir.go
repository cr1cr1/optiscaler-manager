// Package pickdir asks the OS for a directory using the available native
// dialog. go-shirei has no native dialogs, so the desktop's own chooser is
// driven from each platform:
//
//   - Linux: zenity, then kdialog (must be on PATH).
//   - Windows: PowerShell + Windows Forms FolderBrowserDialog (always
//     available on Windows 7+).
//   - macOS: osascript (always available).
//
// Cancelled dialogs return ("", nil). Platforms pick their picker in
// pickdir_{linux,windows,darwin}.go; this file holds the shared error.
package pickdir

import "errors"

// ErrUnavailable means no supported directory-picker tool was found on PATH.
// Each platform's Pick returns it when its required command is missing.
var ErrUnavailable = errors.New("no directory picker available")
