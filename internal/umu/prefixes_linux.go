//go:build linux

package umu

import (
	"context"
	"os"
	"path/filepath"
)

// prefixContainer is a directory whose children may be wine prefixes.
// pfx marks Steam's compatdata layout, where the child holds the prefix
// in <child>/pfx.
type prefixContainer struct {
	root   string
	source string
	pfx    bool
}

// FindPrefixes scans the known prefix locations for existing wine
// prefixes (directories containing a drive_c): Heroic's
// ~/Games/Heroic/Prefixes (native and Flatpak), Bottles' bottles/
// (XDG and Flatpak), every Steam root and extra library's
// steamapps/compatdata, and the ~/.wine singleton. Missing dirs are
// skipped; entries resolving to the same directory through symlinks are
// deduplicated. Lutris has no canonical prefix root (its prefixes are
// per-game, usually the install dir itself) — those launches are
// covered by the enclosing-drive_c rule in PrefixForGame instead.
//
// The context is reserved for future cancellation; the current
// implementation is purely file I/O.
func FindPrefixes(_ context.Context) []Prefix {
	home, xdgData := homeAndXDGData()

	var containers []prefixContainer
	if home != "" {
		containers = append(containers,
			prefixContainer{filepath.Join(home, "Games", "Heroic", "Prefixes"), "heroic", false},
			prefixContainer{filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "Games", "Heroic", "Prefixes"), "heroic", false},
			prefixContainer{filepath.Join(home, ".var", "app", "com.usebottles.bottles", "data", "bottles", "bottles"), "bottles", false},
		)
	}
	if xdgData != "" {
		containers = append(containers,
			prefixContainer{filepath.Join(xdgData, "bottles", "bottles"), "bottles", false},
		)
	}
	for _, r := range steamLibraryRoots(home, xdgData) {
		containers = append(containers,
			prefixContainer{filepath.Join(r, "steamapps", "compatdata"), "steam", true},
		)
	}

	var out []Prefix
	seen := map[string]bool{}
	add := func(name, path, source string) {
		if !isWinePrefix(path) {
			return
		}
		key := path
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			key = resolved
		}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Prefix{Name: name, Path: path, Source: source})
	}
	for _, c := range containers {
		entries, err := os.ReadDir(c.root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(c.root, e.Name())
			if c.pfx {
				dir = filepath.Join(dir, "pfx")
			}
			add(e.Name(), dir, c.source)
		}
	}
	if home != "" {
		add("default", filepath.Join(home, ".wine"), "wine")
	}
	return out
}

// isWinePrefix reports whether dir looks like a wine prefix (it has a
// drive_c directory inside).
func isWinePrefix(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "drive_c"))
	return err == nil && st.IsDir()
}
