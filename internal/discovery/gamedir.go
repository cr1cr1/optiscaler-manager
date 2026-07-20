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
// rather than a separate game. A child with one of these names never counts
// as a game-bearing child of its parent — the executables inside it belong
// to the parent. The set is intentionally small and lowercase-compared.
var engineFolderNames = map[string]bool{
	"bin": true, "binaries": true,
	"win64": true, "win32": true, "x64": true, "x86": true, "x86_64": true,
	"engine": true, "redist": true, "redistributable": true, "_commonredist": true,
	"support": true, "tools": true, "lib": true, "libs": true,
	"thirdparty": true, "third_party": true, "plugins": true,
	"content": true, "data": true, "resources": true, "assets": true,
	"vendor": true, "runtime": true, "runtimes": true,
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
//   - an exe directly inside dir → GameDirGame;
//   - an exe at depth ≤ maxExeDepth with no game-bearing or
//     container-bearing child in between → GameDirGame (the exe is this
//     dir's own, e.g. bin/game.exe or Binaries/Win64/game.exe);
//   - any child that is itself a game (and not an engine folder by name) or
//     itself a container → GameDirContainer (the dir is a library root);
//   - otherwise → GameDirEmpty.
//
// The engine-folder name list is what separates "bin" (this game's
// binaries) from a same-shaped game folder one level down in a library
// root. Directories already visited through other paths (symlink loops)
// are evaluated once.
func ClassifyGameDir(ctx context.Context, dir string) (GameDirKind, error) {
	return classifyGameDir(ctx, canonicalPath(dir), 0, map[string]bool{})
}

func classifyGameDir(ctx context.Context, dir string, depth int, seen map[string]bool) (GameDirKind, error) {
	if err := ctx.Err(); err != nil {
		return GameDirEmpty, err
	}
	if depth > maxClassifyDepth || seen[dir] {
		return GameDirEmpty, nil
	}
	seen[dir] = true
	own, err := findMainExeWithin(ctx, dir, 0)
	if err != nil {
		return GameDirEmpty, err
	}
	if own != "" {
		return GameDirGame, nil
	}
	deep, err := findMainExeWithin(ctx, dir, maxExeDepth)
	if err != nil {
		return GameDirEmpty, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		// An unreadable directory proves nothing either way; treat it like
		// findMainExe's walk does (skip) instead of failing the whole scan.
		log.Debug().Err(err).Str("dir", dir).Msg("classify: unreadable directory")
		return GameDirEmpty, nil
	}
	gameChildren, containerChildren := 0, 0
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return GameDirEmpty, err
		}
		if strings.HasPrefix(e.Name(), ".") {
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
		case GameDirGame:
			if !engineFolderNames[strings.ToLower(e.Name())] {
				gameChildren++
			}
		case GameDirContainer:
			containerChildren++
		}
		if gameChildren+containerChildren > 0 {
			break // one game-bearing descendant already decides Container
		}
	}
	switch {
	case gameChildren+containerChildren > 0:
		return GameDirContainer, nil
	case deep != "":
		return GameDirGame, nil
	default:
		return GameDirEmpty, nil
	}
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
