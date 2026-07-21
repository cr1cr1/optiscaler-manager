//go:build !windows && !darwin

package discovery

import (
	"io/fs"
	"os"
	"strings"
	"syscall"
)

// fileCandidate accepts regular files that are real executables: a Windows
// PE ("MZ") or an ELF binary, by magic bytes. The extension/exec-bit gate
// runs first (cheap), then the first 4 bytes are sniffed — an execute bit
// alone proves nothing on mounts where every file has one (shader caches,
// appmanifests and scripts were being picked as "game executables").
// Shared libraries, Windows DLLs, and desktop entries stay excluded.
// Returns the file size for ranking.
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
	if info.Mode()&0o111 == 0 && !strings.HasSuffix(lower, ".exe") {
		return false, 0
	}
	if !isBinaryMagic(path) {
		return false, 0
	}
	return true, info.Size()
}

// isBinaryMagic reports whether the file at path starts with the PE ("MZ")
// or ELF magic. Unreadable files are not candidates. The open is
// non-blocking and re-verified regular: a file swapped for a FIFO between
// the walk and the open must never hang the scan.
func isBinaryMagic(path string) bool {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = syscall.Close(fd)
		return false
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	var magic [4]byte
	if _, err := f.ReadAt(magic[:], 0); err != nil {
		return false
	}
	return magic[0] == 'M' && magic[1] == 'Z' ||
		magic[0] == 0x7f && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F'
}

// dirCandidate never matches on unix-likes: only regular files are game
// binaries.
func dirCandidate(path string) (bool, bool) { return false, false }
