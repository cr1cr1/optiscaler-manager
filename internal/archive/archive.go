// Package archive extracts OptiScaler bundle archives (.7z upstream, .zip
// for forks that publish zips) with hostile-input defenses. Third-party
// archives are untrusted: entry names are sanitized before any write, and
// extraction is capped against decompression bombs.
package archive

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
)

// Extraction caps (decompression-bomb defenses). Generous for game-mod
// bundles, fatal for zip-bomb class inputs.
const (
	maxFileSize  = 1 << 30       // 1 GiB per file
	maxTotalSize = 4 * (1 << 30) // 4 GiB per archive
	maxEntries   = 100_000       // entry-count cap
)

// archiveEntry is one member of an opened archive, format-agnostic.
type archiveEntry struct {
	name string
	info fs.FileInfo
	open func() (io.ReadCloser, error)
}

// openEntries opens the archive at path, dispatching on its extension
// (.7z → sevenzip, .zip → stdlib zip), and returns its entries plus a
// close func. Unknown extensions are an error — the format is never
// sniffed, so a mislabeled download fails loud.
func openEntries(path string) ([]archiveEntry, func(), error) {
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".7z":
		zr, err := sevenzip.OpenReader(path)
		if err != nil {
			return nil, nil, fmt.Errorf("open archive %s: %w", path, err)
		}
		entries := make([]archiveEntry, 0, len(zr.File))
		for _, f := range zr.File {
			entries = append(entries, archiveEntry{name: f.Name, info: f.FileInfo(), open: f.Open})
		}
		return entries, func() { _ = zr.Close() }, nil
	case ".zip":
		zr, err := zip.OpenReader(path)
		if err != nil {
			return nil, nil, fmt.Errorf("open archive %s: %w", path, err)
		}
		entries := make([]archiveEntry, 0, len(zr.File))
		for _, f := range zr.File {
			entries = append(entries, archiveEntry{name: f.Name, info: f.FileInfo(), open: f.Open})
		}
		return entries, func() { _ = zr.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("archive %s: unsupported format %q (want .7z or .zip)", path, ext)
	}
}

// List returns the entry names of the archive at path, in archive order.
// Names are returned as stored (slash-separated), unsanitized; callers use
// List for pre-validation only.
func List(path string) ([]string, error) {
	entries, closeFn, err := openEntries(path)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.name)
	}
	return names, nil
}

// ExtractTo extracts the archive at archivePath into dstDir, creating it if
// needed. Unsafe entries (absolute paths, traversal, links, duplicates,
// oversized) abort the extraction with an error naming the offending entry;
// nothing outside dstDir is ever written.
func ExtractTo(archivePath, dstDir string) error {
	entries, closeFn, err := openEntries(archivePath)
	if err != nil {
		return err
	}
	defer closeFn()

	if len(entries) > maxEntries {
		return fmt.Errorf("archive %s: %d entries exceeds cap %d", archivePath, len(entries), maxEntries)
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create staging dir %s: %w", dstDir, err)
	}

	seen := map[string]string{} // case-folded rel path → original name
	var total int64
	for _, e := range entries {
		rel, err := SanitizeName(e.name)
		if err != nil {
			return fmt.Errorf("archive %s: %w", archivePath, err)
		}
		if prev, dup := seen[strings.ToLower(rel)]; dup {
			return fmt.Errorf("archive %s: duplicate entry %q conflicts with %q", archivePath, e.name, prev)
		}
		seen[strings.ToLower(rel)] = e.name

		if e.info.IsDir() {
			if err := os.MkdirAll(filepath.Join(dstDir, rel), 0o755); err != nil {
				return fmt.Errorf("create dir %s: %w", rel, err)
			}
			continue
		}
		if mode := e.info.Mode(); !mode.IsRegular() {
			return fmt.Errorf("archive %s: entry %q is not a regular file (mode %s)", archivePath, e.name, mode)
		}
		if e.info.Size() > maxFileSize {
			return fmt.Errorf("archive %s: entry %q exceeds per-file cap", archivePath, e.name)
		}
		total += e.info.Size()
		if total > maxTotalSize {
			return fmt.Errorf("archive %s: total size exceeds cap", archivePath)
		}
		if err := extractOne(e, filepath.Join(dstDir, rel)); err != nil {
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
func extractOne(e archiveEntry, dest string) error {
	rc, err := e.open()
	if err != nil {
		return fmt.Errorf("open entry %q: %w", e.name, err)
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
		return fmt.Errorf("extract %q: %w", e.name, err)
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
	entries, closeFn, err := openEntries(path)
	if err != nil {
		return "", 0, err
	}
	defer closeFn()

	for _, e := range entries {
		if e.name != entryName {
			continue
		}
		rc, err := e.open()
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
	entries, closeFn, err := openEntries(path)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	var out []string
	for _, e := range entries {
		if e.info.IsDir() || !e.info.Mode().IsRegular() {
			continue
		}
		out = append(out, strings.ToLower(pathpkg.Base(e.name)))
	}
	return out, nil
}
