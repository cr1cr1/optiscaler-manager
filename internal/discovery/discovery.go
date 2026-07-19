// Package discovery finds locally installed Steam games and resolves the
// directory inside a game root where injection DLLs must land.
package discovery

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	govdf "github.com/lewisgibson/go-vdf"
	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// ParseLibraryFolders parses steamapps/libraryfolders.vdf and returns the
// library root paths. Both the modern nested form ("0" { "path" ... }) and
// the legacy flat form ("1" "D:\\SteamLibrary") are accepted.
func ParseLibraryFolders(r io.Reader) ([]string, error) {
	root, err := decodeVDF(r)
	if err != nil {
		return nil, err
	}
	lf := childCI(root, "libraryfolders")
	if lf == nil || lf.Type != govdf.NodeTypeMap {
		return nil, fmt.Errorf("libraryfolders.vdf: missing \"libraryfolders\" key")
	}

	var paths []string
	for _, key := range sortedKeys(lf.Children) {
		child := lf.Children[key]
		switch {
		case child.Type == govdf.NodeTypeMap:
			// Modern form: numbered entry maps carrying a "path" scalar.
			if p := childCI(child, "path"); p != nil && p.Type == govdf.NodeTypeScalar && p.Value != "" {
				paths = append(paths, p.Value)
			}
		case child.Type == govdf.NodeTypeScalar && isNumericKey(key) && child.Value != "":
			// Legacy form: numeric keys map directly to paths; metadata keys
			// (TimeNextStatsReport, ContentStatsID, ...) are non-numeric.
			paths = append(paths, child.Value)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("libraryfolders.vdf: no library paths found")
	}
	return paths, nil
}

// ParseAppmanifest parses one appmanifest_*.acf file and returns the app's
// id, display name, and install directory name (relative to steamapps/common).
func ParseAppmanifest(r io.Reader) (appID, name, installDir string, err error) {
	root, err := decodeVDF(r)
	if err != nil {
		return "", "", "", err
	}
	state := childCI(root, "AppState")
	if state == nil || state.Type != govdf.NodeTypeMap {
		return "", "", "", fmt.Errorf("appmanifest: missing \"AppState\" key")
	}
	appID = scalarCI(state, "appid")
	name = scalarCI(state, "name")
	installDir = scalarCI(state, "installdir")
	if appID == "" || name == "" || installDir == "" {
		return "", "", "", fmt.Errorf("appmanifest: missing required field (appid=%q name=%q installdir=%q)", appID, name, installDir)
	}
	return appID, name, installDir, nil
}

// ScanSteam scans steamRoot and every library listed in its
// steamapps/libraryfolders.vdf for installed games. Unreadable manifests and
// games whose install directory is missing on disk are skipped; an error is
// returned only when no library is readable at all.
func ScanSteam(steamRoot string) ([]domain.Game, error) {
	libs := []string{steamRoot}
	if f, err := os.Open(filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf")); err == nil {
		extra, err := ParseLibraryFolders(f)
		_ = f.Close()
		if err != nil {
			log.Warn().Err(err).Str("root", steamRoot).Msg("libraryfolders.vdf unreadable, scanning root only")
		} else {
			libs = append(libs, extra...)
		}
	}
	libs = dedupePaths(libs)

	var games []domain.Game
	readable := 0
	for _, lib := range libs {
		apps := filepath.Join(lib, "steamapps")
		manifests, err := filepath.Glob(filepath.Join(apps, "appmanifest_*.acf"))
		if err != nil || len(manifests) == 0 {
			if _, statErr := os.Stat(apps); statErr != nil {
				log.Debug().Str("lib", lib).Msg("not a readable Steam library")
				continue
			}
		}
		readable++
		for _, m := range manifests {
			game, ok := readManifest(m, lib)
			if ok {
				games = append(games, game)
			}
		}
	}
	if readable == 0 {
		return nil, fmt.Errorf("no readable Steam library under %s", steamRoot)
	}
	return games, nil
}

// nonGameExclusions are Steam-catalog entries that are not games: runtimes,
// redistributables, tools, and utilities (same classes the reference client
// excludes). Matched case-insensitively as name substrings.
var nonGameExclusions = []string{
	"steamworks common redistributables",
	"steam linux runtime",
	"proton ",
	"proton experimental",
	"steamvr",
	"steam play",
	"steam controller configs",
	"steamworks shared",
	"steam.dll",
	"wallpaper engine",
	"steam deck",
	"steam link",
	"steam sdk",
}

// excludedAppIDs are infrastructure entries best matched by ID.
var excludedAppIDs = map[string]bool{
	"228980": true, // Steamworks Common Redistributables
}

func isNonGame(appID, name string) bool {
	if excludedAppIDs[appID] {
		return true
	}
	lower := strings.ToLower(name)
	for _, pat := range nonGameExclusions {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
}

// readManifest parses one appmanifest and builds the Game, or reports false
// when the manifest is broken, a non-game entry, or the install directory is
// gone.
func readManifest(path, lib string) (domain.Game, bool) {
	f, err := os.Open(path)
	if err != nil {
		log.Warn().Err(err).Str("manifest", path).Msg("skipping unreadable manifest")
		return domain.Game{}, false
	}
	defer func() { _ = f.Close() }()

	appID, name, installDir, err := ParseAppmanifest(f)
	if err != nil {
		log.Warn().Err(err).Str("manifest", path).Msg("skipping broken manifest")
		return domain.Game{}, false
	}
	if isNonGame(appID, name) {
		log.Debug().Str("manifest", path).Str("name", name).Msg("skipping non-game entry")
		return domain.Game{}, false
	}
	dir := filepath.Join(lib, "steamapps", "common", installDir)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		log.Debug().Str("manifest", path).Str("dir", dir).Msg("skipping game with missing install dir")
		return domain.Game{}, false
	}
	return domain.Game{AppID: appID, Name: name, InstallDir: dir, LibraryPath: lib}, true
}

// SteamRoots returns existing Steam installation roots on Linux: native,
// Flatpak, and Snap locations, deduplicated through symlinks.
func SteamRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	candidates := []string{
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam"),
		filepath.Join(home, "snap", "steam", "common", ".steam", "steam"),
	}
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

func decodeVDF(r io.Reader) (*govdf.Node, error) {
	var root govdf.Node
	if err := govdf.NewDecoder(r).Decode(&root); err != nil {
		return nil, fmt.Errorf("parse VDF: %w", err)
	}
	return &root, nil
}

// childCI returns the child of n whose key matches name case-insensitively.
func childCI(n *govdf.Node, name string) *govdf.Node {
	if n == nil || n.Type != govdf.NodeTypeMap {
		return nil
	}
	for k, v := range n.Children {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return nil
}

// scalarCI returns the scalar value of the named child, or "".
func scalarCI(n *govdf.Node, name string) string {
	c := childCI(n, name)
	if c == nil || c.Type != govdf.NodeTypeScalar {
		return ""
	}
	return c.Value
}

func isNumericKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]*govdf.Node) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func dedupePaths(paths []string) []string {
	seen := map[string]bool{}
	out := paths[:0]
	for _, p := range paths {
		key := filepath.Clean(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}
