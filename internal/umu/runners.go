package umu

import "context"

// Runner represents a locally-installed Proton-compatible runner found
// on disk. Path points at the runner directory itself; Source is one of
// "steam", "bottles", "lutris", "heroic", "umu" ("other" when
// unrecognized) depending on which root it came from. Cross-platform so
// the settings frontends can render the picker everywhere (off-Linux
// the scan stubs return nothing).
type Runner struct {
	Name    string
	Path    string
	Source  string
	Version string // normalized to "major.minor.patch" so it sorts sanely
}

// ResolveProtonPath picks the Proton build a launch should use. A
// non-empty pinned path wins verbatim. Empty means Auto, guided by
// preferSource: when the launch's prefix belongs to a launcher (its
// FindPrefixes source), the newest runner OF THAT LAUNCHER wins so a
// Bottles prefix runs on Bottles' runner, not whatever is newest
// system-wide (issue 059). Launchers without runner roots
// (PlayOnLinux, CrossOver, ~/.wine, undetected prefixes) and Auto
// prefixes fall back to the global newest; when nothing is detected the
// result stays empty and umu-run falls back to its own managed runtime
// (UMU-Latest).
func ResolveProtonPath(ctx context.Context, pinned, preferSource string) string {
	if pinned != "" {
		return pinned
	}
	runners := FindRunners(ctx, RunnerRoots())
	if len(runners) == 0 {
		return ""
	}
	if preferSource != "" {
		for _, r := range runners { // newest first
			if r.Source == preferSource {
				return r.Path
			}
		}
	}
	return runners[0].Path
}
