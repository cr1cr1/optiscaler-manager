package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// hashFile streams the file through SHA-256 and returns the hex digest.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFile copies src to dst (creating parents, 0644) and returns the SHA-256
// of the written bytes, verified against the source hash.
func copyFile(src, dst string) (string, error) {
	srcHash, err := hashFile(src)
	if err != nil {
		return "", fmt.Errorf("hash source %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return "", fmt.Errorf("copy %s → %s: %w", src, dst, err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("close %s: %w", dst, err)
	}
	dstHash, err := hashFile(dst)
	if err != nil {
		return "", err
	}
	if dstHash != srcHash {
		return "", fmt.Errorf("copy %s → %s: written bytes failed verification", src, dst)
	}
	return dstHash, nil
}

// canonicalPath resolves dir to an absolute, symlink-free, clean path.
func canonicalPath(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", dir, err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved, nil
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("stat %s: %w", abs, err)
	}
	return abs, nil
}
