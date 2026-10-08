//go:build linux

package umu

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// RunnerRoots returns the existing directories worth scanning for
// Proton-compatible runners: Steam (native, Flatpak, Snap, plus extra
// library folders from steamapps/libraryfolders.vdf), Bottles, Lutris,
// Heroic, and umu's own compatibilitytools. Missing dirs are skipped,
// and entries that resolve to the same directory through symlinks are
// deduplicated. XDG_DATA_HOME / XDG_CONFIG_HOME overrides are honored.
// homeAndXDGData returns HOME and the effective XDG data home
// (XDG_DATA_HOME, defaulting to ~/.local/share).
func homeAndXDGData() (home, xdgData string) {
	home = os.Getenv("HOME")
	xdgData = os.Getenv("XDG_DATA_HOME")
	if xdgData == "" && home != "" {
		xdgData = filepath.Join(home, ".local", "share")
	}
	return home, xdgData
}

func RunnerRoots() []string {
	home, xdgData := homeAndXDGData()
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" && home != "" {
		xdgConfig = filepath.Join(home, ".config")
	}

	var candidates []string
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".steam", "steam", "compatibilitytools.d"),
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam", "compatibilitytools.d"),
			filepath.Join(home, "snap", "steam", "common", ".steam", "steam", "compatibilitytools.d"),
			filepath.Join(home, ".var", "app", "com.usebottles.bottles", "data", "bottles", "runners"),
			filepath.Join(home, ".var", "app", "net.lutris.Lutris", "data", "lutris", "runners", "wine"),
			filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic", "tools", "proton"),
			filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic", "tools", "wine"),
		)
	}
	if xdgData != "" {
		candidates = append(candidates,
			filepath.Join(xdgData, "Steam", "compatibilitytools.d"),
			filepath.Join(xdgData, "bottles", "runners"),
			filepath.Join(xdgData, "lutris", "runners", "wine"),
			filepath.Join(xdgData, "umu", "compatibilitytools"),
		)
	}
	if xdgConfig != "" {
		candidates = append(candidates,
			filepath.Join(xdgConfig, "heroic", "tools", "proton"),
			filepath.Join(xdgConfig, "heroic", "tools", "wine"),
		)
	}
	candidates = append(candidates, steamLibraryCompatTools(home, xdgData)...)

	var roots []string
	seen := map[string]bool{}
	for _, c := range candidates {
		st, err := os.Stat(c)
		if err != nil || !st.IsDir() {
			continue
		}
		key := c
		if resolved, err := filepath.EvalSymlinks(c); err == nil {
			key = resolved
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		roots = append(roots, c)
	}
	return roots
}

// steamLibraryRoots returns the Steam root directory candidates
// (native, Flatpak, Snap, XDG data) plus every extra library folder
// recorded in each root's steamapps/libraryfolders.vdf. Candidates are
// not filtered for existence; callers stat what they need.
func steamLibraryRoots(home, xdgData string) []string {
	var steamDirs []string
	if home != "" {
		steamDirs = append(steamDirs,
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam"),
			filepath.Join(home, "snap", "steam", "common", ".steam", "steam"),
		)
	}
	if xdgData != "" {
		steamDirs = append(steamDirs, filepath.Join(xdgData, "Steam"))
	}
	out := append([]string{}, steamDirs...)
	for _, dir := range steamDirs {
		data, err := os.ReadFile(filepath.Join(dir, "steamapps", "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		out = append(out, libraryFolderPaths(string(data))...)
	}
	return out
}

// steamLibraryCompatTools returns <root-or-library>/compatibilitytools.d
// for every Steam root candidate and extra library folder.
func steamLibraryCompatTools(home, xdgData string) []string {
	var out []string
	for _, r := range steamLibraryRoots(home, xdgData) {
		out = append(out, filepath.Join(r, "compatibilitytools.d"))
	}
	return out
}

// libraryPathRe extracts the "path" values from libraryfolders.vdf.
// The VDF is line-oriented enough in practice that a regex beats pulling
// in a parser; escaped backslashes (Windows-style paths) are collapsed.
var libraryPathRe = regexp.MustCompile(`"path"\s+"((?:[^"\\]|\\.)*)"`)

func libraryFolderPaths(vdf string) []string {
	var out []string
	for _, m := range libraryPathRe.FindAllStringSubmatch(vdf, -1) {
		out = append(out, strings.ReplaceAll(m[1], `\\`, `\`))
	}
	return out
}

// FindRunners scans the supplied root directories for Proton runners.
// Each direct child of a root that passes runnerValid is considered a
// valid runner. Roots that don't exist are skipped silently.
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
			if !runnerValid(dir) {
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
// source names ("steam", "bottles", "lutris", "heroic", "umu") for
// labelling. Anything unrecognized is "other".
func rootSource(root string) string {
	r := filepath.Clean(root)
	switch {
	case strings.HasSuffix(r, "compatibilitytools.d"):
		// Steam is the only launcher using compatibilitytools.d (native,
		// Flatpak, Snap, and extra library folders alike).
		return "steam"
	case strings.HasSuffix(r, filepath.Join("bottles", "runners")):
		return "bottles"
	case strings.HasSuffix(r, filepath.Join("lutris", "runners", "wine")):
		return "lutris"
	case strings.HasSuffix(r, filepath.Join("heroic", "tools", "proton")),
		strings.HasSuffix(r, filepath.Join("heroic", "tools", "wine")):
		return "heroic"
	case strings.HasSuffix(r, filepath.Join("umu", "compatibilitytools")):
		return "umu"
	default:
		return "other"
	}
}

// runnerValid reports whether dir looks like a usable Proton build:
// either the Steam-style toolmanifest.vdf marker, or the top-level
// proton script that GE-Proton repacks for Lutris/Heroic ship (umu's
// PROTONPATH needs it). Plain wine builds (bin/wine only) are rejected.
func runnerValid(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "toolmanifest.vdf")); err == nil {
		return true
	}
	st, err := os.Stat(filepath.Join(dir, "proton"))
	return err == nil && !st.IsDir()
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
