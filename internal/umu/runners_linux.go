//go:build linux

package umu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Runner represents a locally-installed Proton-compatible runner found
// on disk. Path points at the runner directory itself; Source is one of
// "steam", "bottles", "umu" depending on which root it came from.
type Runner struct {
	Name    string
	Path    string
	Source  string
	Version string // normalized to "major.minor.patch" so it sorts sanely
}

// DefaultRootCandidates returns the conventional scan locations:
// $XDG_DATA_HOME/Steam/compatibilitytools.d, $XDG_DATA_HOME/bottles/runners,
// $XDG_DATA_HOME/umu/compatibilitytools. Missing dirs are silently
// skipped by FindRunners, so returning all three unconditionally is fine.
func DefaultRootCandidates() []string {
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		xdg = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	return []string{
		filepath.Join(xdg, "Steam", "compatibilitytools.d"),
		filepath.Join(xdg, "bottles", "runners"),
		filepath.Join(xdg, "umu", "compatibilitytools"),
	}
}

// FindRunners scans the supplied root directories for Proton runners.
// Each direct child of a root that contains a toolmanifest.vdf is
// considered a valid runner. Roots that don't exist are skipped silently.
// Results are sorted by Version descending (newest first); equal versions
// preserve the order of roots supplied.
//
// The context is reserved for future cancellation; current
// implementation is purely file I/O.
func FindRunners(_ context.Context, roots []string) []Runner {
	type tagged struct {
		r      Runner
		rootIx int
	}
	var all []tagged
	for i, root := range roots {
		source := rootSource(root)
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(root, e.Name())
			if !hasManifest(dir) {
				continue
			}
			all = append(all, tagged{
				r: Runner{
					Name:    e.Name(),
					Path:    dir,
					Source:  source,
					Version: parseRunnerVersion(e.Name()),
				},
				rootIx: i,
			})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		return versionLess(all[j].r.Version, all[i].r.Version) // desc
	})
	out := make([]Runner, len(all))
	for k, t := range all {
		out[k] = t.r
	}
	return out
}

// rootSource classifies a root directory path into one of the canonical
// source names ("steam", "bottles", "umu") for labelling. Anything
// unrecognized is "other".
func rootSource(root string) string {
	r := filepath.Clean(root)
	switch {
	case strings.HasSuffix(r, filepath.Join("Steam", "compatibilitytools.d")):
		return "steam"
	case strings.HasSuffix(r, filepath.Join("bottles", "runners")):
		return "bottles"
	case strings.HasSuffix(r, filepath.Join("umu", "compatibilitytools")):
		return "umu"
	default:
		return "other"
	}
}

// hasManifest reports whether dir contains a toolmanifest.vdf file (the
// presence marker Steam / umu / Bottles all use for valid Proton builds).
func hasManifest(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "toolmanifest.vdf"))
	return err == nil
}

// versionNum matches a numeric version component. We use a regex rather
// than splitting on "." because runner names embed versions in mixed
// separators ("-Proton9-3" -> "9.3", "-10.0-1" -> "10.0.1").
var versionNum = regexp.MustCompile(`\d+`)

// preReleaseTail strips a trailing -rc/-beta/-alpha/-pre token from the
// name so its embedded number doesn't get parsed as a real version
// component (e.g. "UMU-Proton-10.0-rc1" -> "UMU-Proton-10.0").
var preReleaseTail = regexp.MustCompile(`-(?:rc|beta|alpha|pre)\d*$`)

// parseRunnerVersion extracts numeric components from a runner name and
// returns them joined by ".". Up to three numeric runs are used; extras
// are ignored. Strings with no digits return "0" so they sort below
// everything else. The result is not padded: "9.3" stays "9.3" (the
// versionLess comparator treats missing trailing components as 0).
func parseRunnerVersion(name string) string {
	stripped := preReleaseTail.ReplaceAllString(name, "")
	parts := versionNum.FindAllString(stripped, 3)
	if len(parts) == 0 {
		return "0"
	}
	// Trim leading zeros so "09" -> "9" for stable comparison.
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

// versionLess reports whether a < b under dotted-numeric comparison.
// Components are compared as integers, missing components treated as 0.
func versionLess(a, b string) bool {
	av := strings.Split(a, ".")
	bv := strings.Split(b, ".")
	n := len(av)
	if len(bv) > n {
		n = len(bv)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(av) {
			ai, _ = strconv.Atoi(av[i])
		}
		if i < len(bv) {
			bi, _ = strconv.Atoi(bv[i])
		}
		if ai != bi {
			return ai < bi
		}
	}
	return false
}

// (no fmt use yet, reserved for diagnostic logging later)
var _ = fmt.Sprintf
