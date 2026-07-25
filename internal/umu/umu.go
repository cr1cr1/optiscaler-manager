// Package umu integrates the umu-launcher project
// (https://github.com/Open-Wine-Components/umu-launcher) for launching
// Windows binaries on Linux via Proton without depending on Steam.
//
// Detection, Proton-runner discovery, and invocation live here. The
// Launch function and its helpers are Linux-only; non-Linux builds get
// the shared error sentinels and types (so cross-platform callers can
// errors.Is against them without their own platform gates) but the
// Detect / FindRunners / Launch calls themselves no-op or return
// ErrUnavailable off-Linux.
package umu

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"
)

// ErrUnavailable is returned on non-Linux platforms, or on Linux when
// umu-run is not on PATH / its --version output is unparseable.
var ErrUnavailable = errors.New("umu-launcher not available")

// ErrMissingExe is returned by Launch when opts.ExePath is empty.
// Defined here (cross-platform) so cross-platform callers can match it.
var ErrMissingExe = errors.New("umu: missing ExePath")

// ErrUmuSetupFailed wraps umu failures detected via the exit-0 quirk
// (umu's __main__ catches BaseException, logs a traceback, and exits 0).
// Defined here (cross-platform) so cross-platform callers can match it.
var ErrUmuSetupFailed = errors.New("umu: launcher setup failed")

// LaunchOpts describes a single umu-run invocation. Cross-platform so
// callers can construct it off-Linux (the actual Launch call is a no-op
// there).
type LaunchOpts struct {
	ExePath    string
	Args       []string
	GameID     string
	WinePrefix string
	ProtonPath string
	Store      string
	UmuRunPath string
	ExtraEnv   map[string]string
}

// versionRegex matches the first three dotted numeric components after
// the literal "version" word. umu-run prints, e.g.,
// "umu-launcher version 1.4.4 (Python 3.11.6)".
var versionRegex = regexp.MustCompile(`version\s+(\d+\.\d+\.\d+)`)

// Detect finds umu-run on PATH and parses its --version output.
//
// Returns (path, version, nil) on success, where path is the absolute
// resolved umu-run path. Returns ("", "", ErrUnavailable) when umu-run
// is missing or its output cannot be parsed. Works on every platform:
// exec.LookPath("umu-run") fails off-Linux, so callers get a uniform
// ErrUnavailable without needing their own platform gate.
func Detect(ctx context.Context) (path, version string, err error) {
	exe, lookupErr := exec.LookPath("umu-run")
	if lookupErr != nil {
		return "", "", ErrUnavailable
	}

	out, runErr := exec.CommandContext(ctx, exe, "--version").Output()
	if runErr != nil {
		return "", "", ErrUnavailable
	}

	m := versionRegex.FindStringSubmatch(strings.TrimSpace(string(out)))
	if m == nil {
		return "", "", ErrUnavailable
	}
	return exe, m[1], nil
}
