package discovery

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// GameDirKind classifies a directory by how game executables sit inside it.
// Callers that add directories manually use the kind to tell a real game
// folder apart from a library root that should be scanned per child, and
// from a directory with nothing executable at all.
type GameDirKind int

const (
	// GameDirEmpty means no executable qualifies within the scanner's depth
	// budget: not a game, and no game-bearing children either.
	GameDirEmpty GameDirKind = iota
	// GameDirGame means the directory itself contains a game: an executable
	// directly inside it, or one deeper down with no game-bearing child in
	// between (engine layouts like Binaries/Win64).
	GameDirGame
	// GameDirContainer means the directory holds other directories that are
	// (or contain) games: it is a library/collection root to be scanned
	// transparently, never added as a game itself.
	GameDirContainer
)

// String renders the kind for logs and diagnostics.
func (k GameDirKind) String() string {
	switch k {
	case GameDirGame:
		return "game"
	case GameDirContainer:
		return "container"
	default:
		return "empty"
	}
}

// LooksLikeGameDir reports whether dir is itself a game directory (see
// GameDirGame). It is the boolean form of ClassifyGameDir.
func LooksLikeGameDir(ctx context.Context, dir string) bool {
	kind, err := ClassifyGameDir(ctx, dir)
	return err == nil && kind == GameDirGame
}

// engineFolderNames are subdirectory names that hold a game's own binaries
// or platform plumbing rather than a separate game. A child with one of
// these names never counts as a game-bearing or container-bearing child of
// its parent — the executables inside it belong to the parent (or to the
// platform). The set is intentionally small and lowercase-compared.
var engineFolderNames = map[string]bool{
	"bin": true, "binaries": true, "bin64": true, "bin32": true,
	"win64": true, "win32": true, "x64": true, "x86": true, "x86_64": true,
	"engine": true, "redist": true, "redistributable": true, "_commonredist": true,
	"support": true, "tools": true, "lib": true, "libs": true,
	"thirdparty": true, "third_party": true, "plugins": true,
	"content": true, "data": true, "resources": true, "assets": true,
	"vendor": true, "runtime": true, "runtimes": true, "retail": true,
	"__installer": true, "_redist": true, "exe": true,
	// Wine prefixes and Steam's steamapps plumbing hold platform runtime
	// files, never standalone games (a Proton game's files live in
	// common/<Game>; the prefix only holds wine's drive_c).
	"drive_c": true, "compatdata": true, "shadercache": true,
	"downloading": true, "temp": true, "music": true, "sourcemods": true,
	"steamworks common redistributables": true, "steamworks shared": true,
	"steamvr": true, "workshop": true,
}

// plumbingFolderNames are the engineFolderNames whose subtrees no exe walk
// may descend at all: platform plumbing that can only ever hold download
// fragments, shader caches, prefixes, runtimes, mods, and third-party
// tooling — never a game's own binary. The remaining engine folders (bin,
// Binaries/Win64, Retail, Engine, …) stay walkable because the game's exe
// legitimately lives inside them.
var plumbingFolderNames = map[string]bool{
	"compatdata": true, "shadercache": true, "downloading": true,
	"temp": true, "music": true, "sourcemods": true, "workshop": true,
	"thirdparty": true, "third_party": true,
	"steamworks common redistributables": true, "steamworks shared": true,
	"steamvr": true,
}

// plumbingWalkDir reports whether the walker should prune dir (a child of
// parentBase): plumbing folders and platform tools (Proton, Steam Linux
// Runtimes) always, and the OS subtrees of a Wine prefix
// (drive_c/windows, drive_c/users) — a prefix's games live under
// Program Files / GOG Games, which stay walkable.
func plumbingWalkDir(name, parentBase string) bool {
	name = strings.ToLower(name)
	if plumbingFolderNames[name] || platformToolName(name) {
		return true
	}
	if (name == "windows" || name == "users") && strings.ToLower(parentBase) == "drive_c" {
		return true
	}
	return false
}

// engineFolderName reports whether name marks a subdirectory that holds a
// game's own binaries or platform plumbing (engineFolderNames plus the
// versioned Proton / Steam Linux Runtime folders, which ship their own
// versioned names). Engine-named directories never become rows and never
// make their parent a container.
func engineFolderName(name string) bool {
	name = strings.ToLower(name)
	if engineFolderNames[name] {
		return true
	}
	return platformToolName(name)
}

// platformToolName reports the name-pattern half of engineFolderName:
// versioned Proton builds and Steam Linux Runtimes.
func platformToolName(name string) bool {
	if name == "proton" || strings.HasPrefix(name, "proton - ") ||
		strings.HasPrefix(name, "proton hotfix") ||
		strings.HasPrefix(name, "proton easyanticheat") ||
		strings.HasPrefix(name, "proton battleye") ||
		(strings.HasPrefix(name, "proton ") && len(name) > 7 && name[7] >= '0' && name[7] <= '9') {
		return true
	}
	return strings.HasPrefix(name, "steamlinuxruntime")
}

// maxClassifyDepth bounds the recursion ClassifyGameDir does while proving
// that no descendant is a game. It only guards pathological trees; real
// libraries nest far less.
const maxClassifyDepth = 6

// ClassifyGameDir sorts dir into GameDirGame, GameDirContainer, or
// GameDirEmpty using only stats and bounded directory walks (no PE
// parsing). Executable candidacy, skip tokens, and the depth cap are
// exactly findMainExe's, so the predicate never disagrees with the scanner
// about what counts as a game binary.
//
// Classification rules:
//   - an engine-named root is Empty (Proton folders, compatdata trees,
//     and other plumbing added directly hold no games of their own);
//   - a platform install (steam.exe and Steam.dll side by side) is always
//     a Container, even with a top-level exe and no game-bearing child —
//     the exe is the launcher, not a game;
//   - any child that is itself a container → GameDirContainer: the dir is
//     a library root, and even an exe of its own does not make it a game
//     (that exe is platform/collection tooling, like a Steam client's
//     steam.exe next to SteamApps);
//   - an exe directly inside dir → GameDirGame (a game child does not
//     change that — the both-case rows the dir and the child);
//   - any child that is itself a game → GameDirContainer;
//   - an exe at depth ≤ maxExeDepth with no game-bearing or
//     container-bearing child in between → GameDirGame (the exe is this
//     dir's own, e.g. bin/game.exe or Binaries/Win64/game.exe);
//   - otherwise → GameDirEmpty.
//
// The engine-folder name list is what separates "bin" (this game's
// binaries) from a same-shaped game folder one level down in a library
// root. Directories already visited through other paths (symlink loops)
// are evaluated once.
func ClassifyGameDir(ctx context.Context, dir string) (GameDirKind, error) {
	dir = canonicalPath(dir)
	if engineFolderName(filepath.Base(dir)) {
		return GameDirEmpty, nil
	}
	return classifyGameDir(ctx, dir, 0, map[string]bool{})
}

func classifyGameDir(ctx context.Context, dir string, depth int, seen map[string]bool) (GameDirKind, error) {
	if err := ctx.Err(); err != nil {
		return GameDirEmpty, err
	}
	if depth > maxClassifyDepth || seen[dir] {
		return GameDirEmpty, nil
	}
	seen[dir] = true
	entries, err := os.ReadDir(dir)
	if err != nil {
		// An unreadable directory proves nothing either way; treat it like
		// findMainExe's walk does (skip) instead of failing the whole scan.
		log.Debug().Err(err).Str("dir", dir).Msg("classify: unreadable directory")
		return GameDirEmpty, nil
	}
	if isPlatformInstall(entries) {
		return GameDirContainer, nil
	}
	own, err := findMainExeWithin(ctx, dir, 0)
	if err != nil {
		return GameDirEmpty, err
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return GameDirEmpty, err
		}
		if strings.HasPrefix(e.Name(), ".") || engineFolderName(e.Name()) {
			continue
		}
		child := dirChild(e, dir)
		if child == "" || seen[canonicalPath(child)] {
			continue
		}
		kind, err := classifyGameDir(ctx, canonicalPath(child), depth+1, seen)
		if err != nil {
			return GameDirEmpty, err
		}
		switch kind {
		case GameDirContainer:
			return GameDirContainer, nil // a library child decides, own exe or not
		case GameDirGame:
			if own == "" {
				return GameDirContainer, nil
			}
			// With an exe of our own the dir rows anyway (both-case); only
			// a container child could still outrank it, so keep looking.
		}
	}
	if own != "" {
		return GameDirGame, nil
	}
	deep, err := findMainExeWithin(ctx, dir, maxExeDepth)
	if err != nil {
		return GameDirEmpty, err
	}
	if deep != "" {
		return GameDirGame, nil
	}
	return GameDirEmpty, nil
}

// isPlatformInstall reports whether entries mark a platform client install
// (a Steam client directory): the launcher exe next to its runtime DLL.
func isPlatformInstall(entries []fs.DirEntry) bool {
	exe, dll := false, false
	for _, e := range entries {
		switch strings.ToLower(e.Name()) {
		case "steam.exe":
			exe = true
		case "steam.dll":
			dll = true
		}
	}
	return exe && dll
}

// dirChild resolves e (a directory entry of dir) to the child path to
// classify, following symlinks to directories. It returns "" for non-dirs,
// broken links, and links that do not resolve to a directory.
func dirChild(e fs.DirEntry, dir string) string {
	child := filepath.Join(dir, e.Name())
	if e.IsDir() {
		return child
	}
	if e.Type()&fs.ModeSymlink == 0 {
		return ""
	}
	st, err := os.Stat(child)
	if err != nil || !st.IsDir() {
		return ""
	}
	// Resolve the link: findMainExeWithin walks with WalkDir, which does not
	// descend a symlink root, so the unresolved path would count as
	// non-gamey while the rest of the scanner (canonicalizing first) sees
	// the target.
	resolved, err := filepath.EvalSymlinks(child)
	if err != nil {
		return ""
	}
	return resolved
}
