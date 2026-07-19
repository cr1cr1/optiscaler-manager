//go:build darwin

package discovery

import (
	"io/fs"
	"os"
	"strings"
)

// fileCandidate accepts Mach-O binaries (by magic bytes). Returns the file
// size for ranking.
func fileCandidate(path string, d fs.DirEntry) (bool, int64) {
	if skippedBinaryName(d.Name()) {
		return false, 0
	}
	f, err := os.Open(path)
	if err != nil {
		return false, 0
	}
	defer func() { _ = f.Close() }()
	var magic [4]byte
	if _, err := f.ReadAt(magic[:], 0); err != nil {
		return false, 0
	}
	if !isMachO(magic) {
		return false, 0
	}
	info, err := d.Info()
	if err != nil {
		return false, 0
	}
	return true, info.Size()
}

// dirCandidate treats a *.app bundle as one candidate (the bundle path
// itself) and stops the walker from descending into it.
func dirCandidate(path string) (bool, bool) {
	if strings.HasSuffix(strings.ToLower(path), ".app") {
		return true, true
	}
	return false, false
}

func isMachO(magic [4]byte) bool {
	switch magic {
	case [4]byte{0xfe, 0xed, 0xfa, 0xce}, // 32-bit BE
		[4]byte{0xce, 0xfa, 0xed, 0xfe}, // 32-bit LE
		[4]byte{0xfe, 0xed, 0xfa, 0xcf}, // 64-bit BE
		[4]byte{0xcf, 0xfa, 0xed, 0xfe}, // 64-bit LE
		[4]byte{0xca, 0xfe, 0xba, 0xbe}, // universal
		[4]byte{0xca, 0xfe, 0xba, 0xbf}: // universal 64
		return true
	}
	return false
}
