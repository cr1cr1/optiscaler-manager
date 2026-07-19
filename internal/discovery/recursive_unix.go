//go:build !windows && !darwin

package discovery

import (
	"io/fs"
	"strings"
)

// fileCandidate accepts regular files with any execute bit set, excluding
// shared libraries, Windows DLLs, and desktop entries. Returns the file size
// for ranking.
func fileCandidate(path string, d fs.DirEntry) (bool, int64) {
	lower := strings.ToLower(d.Name())
	if strings.HasSuffix(lower, ".dll") || strings.HasSuffix(lower, ".desktop") ||
		strings.HasSuffix(lower, ".so") || strings.Contains(lower, ".so.") {
		return false, 0
	}
	if skippedBinaryName(d.Name()) {
		return false, 0
	}
	info, err := d.Info()
	if err != nil {
		return false, 0
	}
	if info.Mode()&0o111 == 0 {
		return false, 0
	}
	return true, info.Size()
}

// dirCandidate never matches on unix-likes: only regular files are game
// binaries.
func dirCandidate(path string) (bool, bool) { return false, false }
