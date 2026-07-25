package umu

import (
	"os"
	"path/filepath"
	"strings"
)

// windowsBinaryExtensions are filename suffixes that always indicate a
// Windows binary, regardless of the file's actual content.
var windowsBinaryExtensions = []string{".exe", ".bat", ".cmd", ".msi"}

// IsWindowsBinary reports whether path points at a Windows executable
// (or installer). The check is:
//
//  1. If the extension is .exe/.bat/.cmd/.msi, return true. The user
//     explicitly named the file as a Windows binary; if stat fails we
//     still trust the extension rather than refuse launching.
//  2. Otherwise, peek at the file's first two bytes and look for the
//     PE/COFF "MZ" magic header.
//  3. Files that don't exist and have no Windows extension return false.
//
// This is used to decide whether to route a manual-store launch through
// umu-launcher on Linux: a Windows binary needs Proton, a native binary
// doesn't.
func IsWindowsBinary(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, win := range windowsBinaryExtensions {
		if ext == win {
			return true
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 2 {
		return false
	}
	// PE files begin with the IMAGE_DOS_HEADER, whose e_magic field is
	// the two-byte signature 'M','Z' (initials of Mark Zbikowski).
	return data[0] == 'M' && data[1] == 'Z'
}
