//go:build linux

package umu

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LookupPrefix returns the FindPrefixes entry for the prefix at path
// (not-ok when the scan does not know it): the launch hook uses the
// source to pick the runner from the same launcher as the prefix
// (issue 059) and the owner ref/flatpak flag to route the launch to
// the owning launcher's CLI (issue 060).
func LookupPrefix(ctx context.Context, path string) (Prefix, bool) {
	if path == "" {
		return Prefix{}, false
	}
	for _, p := range FindPrefixes(ctx) {
		if p.Path == path {
			return p, true
		}
	}
	return Prefix{}, false
}

// prefixContainer is a directory whose children may be wine prefixes.
// pfx marks Steam's compatdata layout, where the child holds the prefix
// in <child>/pfx and is named after the appmanifest when one exists.
// flatpak marks roots under a Flatpak app's ~/.var/app tree so the
// launch router invokes the Flatpak CLI form (issue 060).
type prefixContainer struct {
	root    string
	source  string
	pfx     bool
	flatpak bool
}

// FindPrefixes scans the known prefix locations for existing wine
// prefixes (directories containing a drive_c): Heroic's
// ~/Games/Heroic/Prefixes (native and Flatpak), Bottles' bottles/
// (XDG and Flatpak) plus the custom_bottles_path root named in
// Bottles' data.yml (issue 053), every Steam root and extra library's
// steamapps/compatdata (named from the sibling appmanifest when
// present), PlayOnLinux's wineprefix/, CrossOver's ~/.cxoffice, the
// per-game prefix: paths in Lutris's YAML configs (XDG config and
// Flatpak), and the ~/.wine singleton. Missing dirs are skipped;
// entries resolving to the same directory through symlinks are
// deduplicated. Lutris games without a custom prefix: default to the
// install dir — those launches are covered by the enclosing-drive_c
// rule in PrefixForGame instead.
//
// The context is reserved for future cancellation; the current
// implementation is purely file I/O.
func FindPrefixes(_ context.Context) []Prefix {
	home, xdgData := homeAndXDGData()
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" && home != "" {
		xdgConfig = filepath.Join(home, ".config")
	}

	var containers []prefixContainer
	if home != "" {
		containers = append(containers,
			prefixContainer{filepath.Join(home, "Games", "Heroic", "Prefixes"), "heroic", false, false},
			prefixContainer{filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "Games", "Heroic", "Prefixes"), "heroic", false, true},
			prefixContainer{filepath.Join(home, ".var", "app", "com.usebottles.bottles", "data", "bottles", "bottles"), "bottles", false, true},
			prefixContainer{filepath.Join(home, ".PlayOnLinux", "wineprefix"), "playonlinux", false, false},
			prefixContainer{filepath.Join(home, ".cxoffice"), "crossover", false, false},
		)
	}
	if xdgData != "" {
		containers = append(containers,
			prefixContainer{filepath.Join(xdgData, "bottles", "bottles"), "bottles", false, false},
		)
	}
	// Bottles with a user-chosen bottles directory: data.yml (next to the
	// default bottles/ root, native or Flatpak) names it via
	// custom_bottles_path, and prefixes live directly under it (issue 053).
	for i, dataYML := range []string{
		filepath.Join(xdgData, "bottles", "data.yml"),
		filepath.Join(home, ".var", "app", "com.usebottles.bottles", "data", "bottles", "data.yml"),
	} {
		if root := bottlesCustomPath(dataYML); root != "" {
			containers = append(containers, prefixContainer{root, "bottles", false, i == 1})
		}
	}
	for _, r := range steamLibraryRoots(home, xdgData) {
		containers = append(containers,
			prefixContainer{filepath.Join(r, "steamapps", "compatdata"), "steam", true, false},
		)
	}

	var out []Prefix
	seen := map[string]bool{}
	add := func(name, path, source, ownerRef string, flatpak bool) {
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
		out = append(out, Prefix{Name: name, Path: path, Source: source, OwnerRef: ownerRef, Flatpak: flatpak})
	}
	// Heroic's GamesConfig maps explicit winePrefix paths to app names
	// for the owner ref (issue 060); unmapped prefixes fall back to the
	// directory name.
	heroicApps := heroicWinePrefixes(home, xdgConfig)
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
			name := e.Name()
			if c.pfx {
				dir = filepath.Join(dir, "pfx")
				if n := appmanifestName(filepath.Dir(c.root), name); n != "" {
					name = n
				}
			}
			ownerRef := ""
			switch c.source {
			case "bottles":
				// The configured bottle name (bottle.yml) — Bottles
				// lowercases dirs but bottles-cli matches names
				// case-sensitively (issue 069); dir name as fallback.
				ownerRef = bottleName(dir)
				if ownerRef == "" {
					ownerRef = e.Name()
				}
			case "heroic":
				ownerRef = heroicApps[dir]
				if ownerRef == "" {
					ownerRef = e.Name()
				}
			}
			add(name, dir, c.source, ownerRef, c.flatpak)
		}
	}
	for _, cfgDir := range lutrisConfigDirs(home, xdgConfig) {
		flatpak := strings.Contains(cfgDir, string(filepath.Separator)+".var"+string(filepath.Separator))
		entries, err := os.ReadDir(cfgDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
				continue
			}
			name, prefix := lutrisYAMLConfig(filepath.Join(cfgDir, e.Name()))
			if prefix == "" {
				continue
			}
			stem := strings.TrimSuffix(e.Name(), ".yml")
			if name == "" {
				name = stem
			}
			add(name, prefix, "lutris", lutrisSlug(stem), flatpak)
		}
	}
	if home != "" {
		add("default", filepath.Join(home, ".wine"), "wine", "", false)
	}
	return out
}

// lutrisConfigDirs returns the Lutris per-game config directories to
// scan: the native one (XDG_CONFIG_HOME/lutris/games) and the Flatpak
// one. Missing dirs are skipped by the caller's ReadDir.
func lutrisConfigDirs(home, xdgConfig string) []string {
	var dirs []string
	if xdgConfig != "" {
		dirs = append(dirs, filepath.Join(xdgConfig, "lutris", "games"))
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".var", "app", "net.lutris.Lutris", "config", "lutris", "games"))
	}
	return dirs
}

// acfNameRe extracts the "name" value from a Steam appmanifest_*.acf.
// The ACF is line-oriented enough in practice that a regex beats
// pulling in a parser (the libraryfolders.vdf precedent).
var acfNameRe = regexp.MustCompile(`"name"\s+"([^"]+)"`)

// appmanifestName returns the game title recorded in
// steamapps/appmanifest_<appid>.acf, "" when unreadable or absent.
func appmanifestName(steamappsDir, appid string) string {
	data, err := os.ReadFile(filepath.Join(steamappsDir, "appmanifest_"+appid+".acf"))
	if err != nil {
		return ""
	}
	if m := acfNameRe.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return ""
}

// lutrisYAMLConfig extracts the top-level name: and the game: prefix:
// from a Lutris per-game YAML config with a line scanner — two flat
// keys don't justify a YAML dependency. Empty prefix means the game
// uses Lutris's default (the install dir).
func lutrisYAMLConfig(path string) (name, prefix string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if name == "" && strings.HasPrefix(trimmed, "name:") {
			name = unquoteYAML(strings.TrimSpace(strings.TrimPrefix(trimmed, "name:")))
		}
		if prefix == "" && strings.HasPrefix(trimmed, "prefix:") {
			prefix = unquoteYAML(strings.TrimSpace(strings.TrimPrefix(trimmed, "prefix:")))
		}
	}
	return name, prefix
}

// bottleName reads the configured bottle name from a bottle's
// bottle.yml (top-level "Name: …" line; quotes stripped). "" when
// unreadable — the caller falls back to the directory name.
func bottleName(bottleDir string) string {
	data, err := os.ReadFile(filepath.Join(bottleDir, "bottle.yml"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if name, ok := strings.CutPrefix(line, "Name:"); ok {
			return strings.Trim(strings.TrimSpace(name), `"'`)
		}
	}
	return ""
}

// BottleRunner reads a bottle's configured runner (bottle.yml
// top-level "Runner: …" line), e.g. for the wineserver cleanup on
// stop (issue 069). "" when unreadable.
func BottleRunner(bottleDir string) string {
	data, err := os.ReadFile(filepath.Join(bottleDir, "bottle.yml"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if runner, ok := strings.CutPrefix(line, "Runner:"); ok {
			return strings.Trim(strings.TrimSpace(runner), `"'`)
		}
	}
	return ""
}

// bottlesCustomPath reads Bottles' data.yml and returns the
// custom_bottles_path value (the user-chosen bottles root), or "" when
// the file or key is absent.
func bottlesCustomPath(dataYML string) string {
	data, err := os.ReadFile(dataYML)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "custom_bottles_path:") {
			return unquoteYAML(strings.TrimSpace(strings.TrimPrefix(trimmed, "custom_bottles_path:")))
		}
	}
	return ""
}

// unquoteYAML strips one layer of matching single or double quotes.
func unquoteYAML(s string) string {
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// isWinePrefix reports whether dir looks like a wine prefix (it has a
// drive_c directory inside).
func isWinePrefix(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "drive_c"))
	return err == nil && st.IsDir()
}

// lutrisSlug derives the Lutris game slug from a per-game config
// filename stem: Lutris names the files <slug>-<unix-timestamp>.yml,
// so a trailing "-<digits>" is stripped when present.
func lutrisSlug(stem string) string {
	if i := strings.LastIndex(stem, "-"); i > 0 {
		if suf := stem[i+1:]; suf != "" {
			digits := true
			for _, r := range suf {
				if r < '0' || r > '9' {
					digits = false
					break
				}
			}
			if digits {
				return stem[:i]
			}
		}
	}
	return stem
}

// heroicWinePrefixes maps explicit winePrefix paths from Heroic's
// GamesConfig (*.json, native and Flatpak config dirs) to app names
// (the json filename stem). Heroic writes flat per-game settings; two
// flat keys don't justify a schema beyond the one field.
func heroicWinePrefixes(home, xdgConfig string) map[string]string {
	out := map[string]string{}
	for _, dir := range []string{
		filepath.Join(xdgConfig, "heroic", "GamesConfig"),
		filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic", "GamesConfig"),
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			var cfg struct {
				WinePrefix string `json:"winePrefix"`
			}
			if json.Unmarshal(data, &cfg) != nil || cfg.WinePrefix == "" {
				continue
			}
			out[cfg.WinePrefix] = strings.TrimSuffix(e.Name(), ".json")
		}
	}
	return out
}
