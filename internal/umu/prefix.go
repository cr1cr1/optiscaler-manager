package umu

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

// PrefixFor returns the deterministic per-game WINEPREFIX path that
// Launch should pass to umu-run. Identity is the install directory
// (same install dir -> same prefix, even if the game's display name
// changes). Two games in different install dirs always get different
// prefixes.
//
// Layout: <state-root>/umu-prefixes/<slug>
// where state-root is OM_DATA_DIR if set (used verbatim — the env var
// is the full root, not a parent), else $XDG_DATA_HOME/optiscaler-manager,
// else ~/.local/share/optiscaler-manager. This matches the production
// store.DefaultRoot precedence (internal/store/store.go:34-44) without
// importing it — the umu package is self-contained.
// slug is the first 12 hex chars of SHA-1(installDir) — short enough
// for a directory name, long enough to avoid collisions in a single
// user's library.
func PrefixFor(installDir, _ string) (string, error) {
	root, err := stateRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "umu-prefixes", Slug(installDir)), nil
}

// Slug returns the directory-safe slug derived from installDir. It is
// exported for diagnostic logging (so users can match a prefix dir on
// disk back to a game install path without reproducing the hash).
func Slug(installDir string) string {
	h := sha1.Sum([]byte(installDir))
	return hex.EncodeToString(h[:])[:12]
}

// stateRoot mirrors the production precedence (cmd/deps.go:33-40 +
// store.DefaultRoot): OM_DATA_DIR is the full root and is used verbatim;
// the XDG/HOME fallbacks append /optiscaler-manager themselves.
func stateRoot() (string, error) {
	if v := os.Getenv("OM_DATA_DIR"); v != "" {
		return v, nil
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "optiscaler-manager"), nil
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "share", "optiscaler-manager"), nil
	}
	return "", errors.New("umu: cannot determine state root (set HOME, XDG_DATA_HOME, or OM_DATA_DIR)")
}
