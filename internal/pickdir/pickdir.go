// Package pickdir asks the OS for a directory (Pick) or an image file
// (PickFile, issue 028's poster upload) using the available native
// dialog. go-shirei has no native dialogs, so the desktop's own chooser
// is driven from each platform:
//
//   - Linux: zenity, then kdialog (must be on PATH).
//   - Windows: IFileOpenDialog via COM (Vista+; FOS_PICKFOLDERS for
//     directories, FOS_FILEMUSTEXIST for files).
//   - macOS: osascript (always available).
//
// Cancelled dialogs return ("", nil). Platforms pick their picker in
// pickdir_{linux,windows,darwin}.go / pickfile_<GOOS>.go; this file
// holds the shared error.
package pickdir

import "errors"

// ErrUnavailable means no supported picker tool was found on PATH.
// Each platform's Pick/PickFile returns it when its required command is
// missing.
var ErrUnavailable = errors.New("no directory picker available")
