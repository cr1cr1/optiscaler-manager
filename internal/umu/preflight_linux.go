//go:build linux

package umu

import (
	"fmt"
	"os"
	"path/filepath"
)

// Preflight checks for umu launches (issue 065): failing fast with an
// error naming the broken piece beats a Python traceback deep inside
// umu-run (or a multi-minute hang on a first-run download with no
// network).

// CheckProton validates a Proton build path: empty means umu's own
// default (always valid); a pinned/auto-detected path must be a
// runner directory with an executable proton script.
func CheckProton(path string) error {
	if path == "" {
		return nil
	}
	script := filepath.Join(path, "proton")
	st, err := os.Stat(script)
	if err != nil {
		return fmt.Errorf("proton runner %s has no proton script — pick another runner or leave Auto: %w", path, err)
	}
	if st.IsDir() || st.Mode()&0o111 == 0 {
		return fmt.Errorf("proton runner %s: proton script is not executable — pick another runner or leave Auto", path)
	}
	return nil
}

// CheckPrefixWritable verifies umu will be able to write the prefix:
// the prefix itself when it exists, else its deepest existing
// ancestor (app-managed prefixes are created on first launch). umu
// creates lock files at the prefix root and fails with an OSError
// deep inside Python when it cannot.
func CheckPrefixWritable(prefix string) error {
	dir := prefix
	for {
		st, err := os.Stat(dir)
		if err == nil && st.IsDir() {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return fmt.Errorf("prefix %s has no existing parent directory", prefix)
		}
		dir = parent
	}
	probe, err := os.CreateTemp(dir, ".om-write-probe-*")
	if err != nil {
		return fmt.Errorf("prefix %s is not writable — umu cannot manage it: %w", prefix, err)
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return nil
}

// CheckExe verifies the game executable still exists — a stale
// ExePath otherwise fails inside wine with a generic error.
func CheckExe(exe string) error {
	st, err := os.Stat(exe)
	if err != nil || st.IsDir() {
		return fmt.Errorf("game executable %s not found — rescan the game", exe)
	}
	return nil
}

// RuntimePresent reports whether umu's Steam Linux Runtime is already
// downloaded ($XDG_DATA_HOME/umu/steamrt3 or ~/.local/share/umu/
// steamrt3). Without it the first launch downloads the runtime and
// needs network.
func RuntimePresent() bool {
	base, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		base = xdg
	} else {
		base = filepath.Join(base, ".local", "share")
	}
	st, err := os.Stat(filepath.Join(base, "umu", "steamrt3"))
	return err == nil && st.IsDir()
}
