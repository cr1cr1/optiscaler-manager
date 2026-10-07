package covers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif" // user-picked posters may be GIFs (shirei renders them)
	"os"
	"path/filepath"
	"strings"
)

// overrideName derives the stable cache filename for a directory's
// user-uploaded poster: re-uploading replaces the same file, so the
// settings map (dir → name) never churns.
func overrideName(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return "override_" + hex.EncodeToString(sum[:])[:16] + ".img"
}

// SetOverride copies a user-picked poster into the cache and returns its
// cache filename. The source is only read (never moved or modified), the
// bytes must decode as an image, and the cached copy is normalized to the
// 2:3 aspect invariant like any fetched art (issue 025). The dir must be
// the caller-canonicalized install dir; it only feeds the filename hash.
func (c *Covers) SetOverride(dir, srcPath string) (string, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("cover override: read: %w", err)
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("cover override: %s is not a decodable image", filepath.Base(srcPath))
	}
	if err := c.ensureCacheDir(); err != nil {
		return "", err
	}
	name := overrideName(dir)
	dst := filepath.Join(c.cacheDir, name)
	tmp, err := os.CreateTemp(c.cacheDir, ".ovr-*")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return "", err
	}
	normalizeCover(dst) // aspect invariant (issue 025)
	return name, nil
}

// OverridePath resolves a cache filename recorded in settings to its
// on-disk path, reporting false when the file is gone (a deleted cache
// self-heals by falling back to the fetch chain). Only override_*.img
// basenames resolve — a settings file is user-editable, so nothing else
// may be addressed through it.
func (c *Covers) OverridePath(name string) (string, bool) {
	base := filepath.Base(name)
	if !strings.HasPrefix(base, "override_") || !strings.HasSuffix(base, ".img") {
		return "", false
	}
	p := filepath.Join(c.cacheDir, base)
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}

// ClearOverride removes a cached user poster; missing files are fine.
func (c *Covers) ClearOverride(name string) {
	base := filepath.Base(name)
	if !strings.HasPrefix(base, "override_") || !strings.HasSuffix(base, ".img") {
		return
	}
	_ = os.Remove(filepath.Join(c.cacheDir, base))
}
