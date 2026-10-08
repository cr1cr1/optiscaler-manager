package umu

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Prefix represents a locally-installed wine prefix found on disk. Path
// points at the prefix root (the directory containing drive_c); Source
// is one of "steam", "bottles", "heroic", "wine" ("other" when
// unrecognized). Cross-platform so the settings frontends can render
// the picker everywhere (off-Linux the scan stubs return nothing).
type Prefix struct {
	Name   string
	Path   string
	Source string
}

// ResolvePrefix returns the WINEPREFIX a launch should use: the pinned
// prefix verbatim when set (the caller resolves the precedence —
// per-game pin first, then the global default), otherwise the auto
// chain of PrefixForGame.
func ResolvePrefix(pinned, installDir, exePath, appName string) (string, error) {
	if pinned != "" {
		return pinned, nil
	}
	return PrefixForGame(installDir, exePath, appName)
}

// PrefixForGame resolves the WINEPREFIX a launch should use, preferring
// prefixes that already exist on disk over the app-managed one:
//
//  1. If exePath lives under an existing <prefix>/drive_c (a Bottles,
//     Lutris, or Heroic bottle the game was installed into), that prefix
//     is reused so saves and config carry over.
//  2. If appName is set and a Heroic prefix (native or Flatpak) already
//     exists for it, that prefix is reused.
//  3. Otherwise the deterministic app-managed PrefixFor(installDir) is
//     used (created by umu-run on first launch).
//
// Launcher directories are only ever READ — nothing is created or
// migrated inside them.
func PrefixForGame(installDir, exePath, appName string) (string, error) {
	if p := enclosingPrefix(exePath); p != "" {
		return p, nil
	}
	if p := heroicPrefix(appName); p != "" {
		return p, nil
	}
	return PrefixFor(installDir, "")
}

// enclosingPrefix returns the wine prefix root containing exePath when
// the path walks through a drive_c component whose parent exists on
// disk. "" when the exe is not inside a prefix or the root is missing.
func enclosingPrefix(exePath string) string {
	if exePath == "" {
		return ""
	}
	dir := filepath.Dir(exePath)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		if strings.EqualFold(filepath.Base(dir), "drive_c") {
			if st, err := os.Stat(parent); err == nil && st.IsDir() {
				return parent
			}
			return ""
		}
		dir = parent
	}
}

// heroicPrefix returns the existing Heroic-managed prefix for appName,
// checking the native and Flatpak default locations. "" when appName is
// empty, HOME is unknown, or no prefix directory exists.
func heroicPrefix(appName string) string {
	if appName == "" {
		return ""
	}
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	for _, root := range []string{
		filepath.Join(home, "Games", "Heroic", "Prefixes"),
		filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "Games", "Heroic", "Prefixes"),
	} {
		p := filepath.Join(root, appName)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return ""
}

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
