// Package archive extracts OptiScaler bundle archives (.7z) with hostile-input
// defenses. Third-party archives are untrusted: entry names are sanitized
// before any write, and extraction is capped against decompression bombs.
package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
)

// Extraction caps (decompression-bomb defenses). Generous for game-mod
// bundles, fatal for zip-bomb class inputs.
const (
	maxFileSize  = 1 << 30          // 1 GiB per file
	maxTotalSize = 4 * (1 << 30)    // 4 GiB per archive
	maxEntries   = 100_000          // entry-count cap
)

// List returns the entry names of the archive at path, in archive order.
// Names are returned as stored (slash-separated), unsanitized; callers use
// List for pre-validation only.
func List(path string) ([]string, error) {
	zr, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open archive %s: %w", path, err)
	}
	defer func() { _ = zr.Close() }()

	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names, nil
}

// ExtractTo extracts the archive at archivePath into dstDir, creating it if
// needed. Unsafe entries (absolute paths, traversal, links, duplicates,
// oversized) abort the extraction with an error naming the offending entry;
// nothing outside dstDir is ever written.
func ExtractTo(archivePath, dstDir string) error {
	zr, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open archive %s: %w", archivePath, err)
	}
	defer func() { _ = zr.Close() }()

	if len(zr.File) > maxEntries {
		return fmt.Errorf("archive %s: %d entries exceeds cap %d", archivePath, len(zr.File), maxEntries)
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create staging dir %s: %w", dstDir, err)
	}

	seen := map[string]string{} // case-folded rel path → original name
	var total int64
	for _, f := range zr.File {
		rel, err := SanitizeName(f.Name)
		if err != nil {
			return fmt.Errorf("archive %s: %w", archivePath, err)
		}
		if prev, dup := seen[strings.ToLower(rel)]; dup {
			return fmt.Errorf("archive %s: duplicate entry %q conflicts with %q", archivePath, f.Name, prev)
		}
		seen[strings.ToLower(rel)] = f.Name

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(dstDir, rel), 0o755); err != nil {
				return fmt.Errorf("create dir %s: %w", rel, err)
			}
			continue
		}
		if mode := f.FileInfo().Mode(); !mode.IsRegular() {
			return fmt.Errorf("archive %s: entry %q is not a regular file (mode %s)", archivePath, f.Name, mode)
		}
		if f.FileInfo().Size() > maxFileSize {
			return fmt.Errorf("archive %s: entry %q exceeds per-file cap", archivePath, f.Name)
		}
		total += f.FileInfo().Size()
		if total > maxTotalSize {
			return fmt.Errorf("archive %s: total size exceeds cap", archivePath)
		}
		if err := extractOne(f, filepath.Join(dstDir, rel)); err != nil {
			return err
		}
	}
	return nil
}

// SanitizeName validates an archive entry name and returns its clean,
// slash-native relative path. Rejected: empty names, absolute paths, drive
// letters, UNC paths, any ".." segment, and backslash trickery.
func SanitizeName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty entry name")
	}
	// Normalize backslashes so Windows-style tricks cannot smuggle separators.
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("entry %q: absolute path", name)
	}
	if strings.HasPrefix(name, "//") {
		return "", fmt.Errorf("entry %q: UNC path", name)
	}
	if len(name) >= 2 && name[1] == ':' {
		return "", fmt.Errorf("entry %q: drive letter", name)
	}
	clean := filepath.FromSlash(name)
	for _, seg := range strings.Split(clean, string(filepath.Separator)) {
		if seg == ".." {
			return "", fmt.Errorf("entry %q: path traversal", name)
		}
	}
	rel := filepath.Clean(clean)
	if rel == "." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("entry %q: invalid path", name)
	}
	return rel, nil
}

// extractOne streams one regular-file entry to disk through a SHA-256 hasher
// (the hash is logged by callers via HashEntry; here we only stream).
func extractOne(f *sevenzip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open entry %q: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create parent of %s: %w", dest, err)
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	if _, err := io.Copy(out, rc); err != nil {
		_ = out.Close()
		return fmt.Errorf("extract %q: %w", f.Name, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dest, err)
	}
	return nil
}

// HashEntry extracts the named entry to memory and returns its SHA-256 hex
// digest and size. Used by the spike test to prove the decompression path
// (including BCJ2-filtered DLLs) end to end.
func HashEntry(path, entryName string) (digest string, size int64, err error) {
	zr, err := sevenzip.OpenReader(path)
	if err != nil {
		return "", 0, fmt.Errorf("open archive %s: %w", path, err)
	}
	defer func() { _ = zr.Close() }()

	for _, f := range zr.File {
		if f.Name != entryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", 0, fmt.Errorf("open entry %q: %w", entryName, err)
		}
		defer func() { _ = rc.Close() }()
		h := sha256.New()
		size, err = io.Copy(h, rc)
		if err != nil {
			return "", 0, fmt.Errorf("read entry %q: %w", entryName, err)
		}
		return hex.EncodeToString(h.Sum(nil)), size, nil
	}
	return "", 0, fmt.Errorf("entry %q not found in %s", entryName, path)
}

// EntryNames is a small helper for tests and validation: base names of all
// regular-file entries, lower-cased.
func EntryNames(path string) ([]string, error) {
	zr, err := sevenzip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open archive %s: %w", path, err)
	}
	defer func() { _ = zr.Close() }()

	var out []string
	for _, f := range zr.File {
		info := f.FileInfo()
		if info.IsDir() || !info.Mode().IsRegular() {
			continue
		}
		out = append(out, strings.ToLower(pathpkg.Base(f.Name)))
	}
	return out, nil
}
