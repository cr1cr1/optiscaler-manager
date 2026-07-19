// Package pever extracts version information from Windows PE binaries
// (upscaler DLLs) without cgo or external dependencies. It provides:
//
//   - FileVersion: normalized version string from a PE's version resource
//     (VS_FIXEDFILEINFO, with a ProductVersion string fallback).
//   - MarketingName: maps a raw DLL version to its marketing name via
//     vendored lookup tables (versionmaps.go).
//   - OptiScalerVersion: determines the OptiScaler version of an install
//     directory via a manifest → log → ini evidence chain.
//
// The PE parser is hand-rolled and treats every input as hostile: all
// reads are bounds-checked and malformed input yields sentinel errors
// (ErrNotPE, ErrNoVersionInfo), never panics.
package pever

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// ErrNotPE is returned when the input is not a valid PE32/PE32+ image
// (bad magic, truncated headers, unsupported optional-header magic).
var ErrNotPE = errors.New("pever: not a PE file")

// ErrNoVersionInfo is returned when the PE is structurally valid but
// carries neither a VS_FIXEDFILEINFO block nor a ProductVersion string.
var ErrNoVersionInfo = errors.New("pever: no version info")

// ErrTooLarge is returned when an input file exceeds its size cap.
var ErrTooLarge = errors.New("pever: file exceeds size cap")

// ErrNotRegular is returned when an input path is not a regular file
// (directory, device, symlink to a special file, ...).
var ErrNotRegular = errors.New("pever: not a regular file")

// maxPEFileSize caps PE reads. Real upscaler DLLs are large (the 0.9.4
// bundle ships a 78 MB libxess.dll), so the cap sits well above that.
const maxPEFileSize = 128 << 20

// maxManifestSize caps manifest.json reads; real manifests are a few KB.
const maxManifestSize = 1 << 20

// Kind selects the upscaler family for MarketingName lookups.
type Kind int

const (
	KindDLSS Kind = iota
	KindFSR
	KindXeSS
)

// FileVersion returns the normalized version string of the PE image at
// path. Normalization: the ProductVersion string wins unless it matches
// the placeholder pattern 1.0 / 1.0.x (then the fixed FILEVERSION quad is
// used); commas become dots; a leading "FSR " prefix is stripped;
// surrounding whitespace and one leading "v" are trimmed.
//
// Reads are bounded: path must be a regular file of at most maxPEFileSize
// bytes (ErrNotRegular / ErrTooLarge otherwise).
func FileVersion(path string) (string, error) {
	data, err := readBounded(path, maxPEFileSize)
	if err != nil {
		return "", err
	}
	fixed, product, err := parsePEVersion(data)
	if err != nil {
		return "", err
	}
	v := normalize(fixed, product)
	if v == "" {
		return "", ErrNoVersionInfo
	}
	return v, nil
}

// readBounded reads path after stat-verifying it is a regular file of at
// most limit bytes; the read itself is capped at limit as well, so a file
// that grows between stat and read is truncated rather than slurped.
func readBounded(path string, limit int64) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("pever: stat %s: %w", path, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("pever: %s: %w", path, ErrNotRegular)
	}
	if fi.Size() > limit {
		return nil, fmt.Errorf("pever: %s (%d bytes): %w", path, fi.Size(), ErrTooLarge)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("pever: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return nil, fmt.Errorf("pever: read %s: %w", path, err)
	}
	return data, nil
}

// oneDotZero matches placeholder product versions (1.0, 1.0.0, 1.0.0.0,
// 1.0.7, ...): vendors ship these while the real version lives in the
// fixed FILEVERSION quad.
var oneDotZero = regexp.MustCompile(`^1\.0(\.\d+)*$`)

func normalize(fixed, product string) string {
	v := strings.TrimSpace(product)
	if v == "" || oneDotZero.MatchString(strings.ReplaceAll(v, ",", ".")) {
		v = strings.TrimSpace(fixed)
	}
	v = strings.ReplaceAll(v, ",", ".")
	if rest, ok := strings.CutPrefix(v, "FSR "); ok {
		v = rest
	}
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = cutVersionSuffix(v)
	return v
}

// cutVersionSuffix keeps only the leading dotted-numeric run of a version
// that starts with a digit, dropping vendor suffixes such as
// "-final (7534ad0)" in "0.9.4-final (7534ad0)".
func cutVersionSuffix(v string) string {
	if v == "" || v[0] < '0' || v[0] > '9' {
		return v
	}
	i := 0
	for i < len(v) && (v[i] == '.' || (v[i] >= '0' && v[i] <= '9')) {
		i++
	}
	return strings.TrimRight(v[:i], ".")
}
