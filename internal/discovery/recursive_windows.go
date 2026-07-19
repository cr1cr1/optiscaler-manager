//go:build windows

package discovery

import (
	"io/fs"
	"strings"
)

// fileCandidate accepts *.exe files only. Returns the file size for ranking.
func fileCandidate(path string, d fs.DirEntry) (bool, int64) {
	if !strings.HasSuffix(strings.ToLower(d.Name()), ".exe") {
		return false, 0
	}
	if skippedBinaryName(d.Name()) {
		return false, 0
	}
	info, err := d.Info()
	if err != nil {
		return false, 0
	}
	return true, info.Size()
}

// dirCandidate never matches on Windows: only .exe files are game binaries.
func dirCandidate(path string) (bool, bool) { return false, false }
