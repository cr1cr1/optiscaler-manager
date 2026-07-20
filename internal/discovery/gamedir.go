package discovery

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	// GameDirGame means the directory itself yields a main executable.
	GameDirGame
	// GameDirContainer means the directory has no executable of its own,
	// but at least one immediate subdirectory looks like a game dir: it is
	// a library/collection root to be scanned per child, not added as one
	// game.
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

// LooksLikeGameDir reports whether dir is itself a game directory: an
// executable qualifies directly inside dir or one level down, or deeper
// within the scanner's depth budget (engine-style layouts such as
// Binaries/Win64). It is the boolean form of ClassifyGameDir.
func LooksLikeGameDir(ctx context.Context, dir string) bool {
	kind, err := ClassifyGameDir(ctx, dir)
	return err == nil && kind == GameDirGame
}

// ClassifyGameDir sorts dir into GameDirGame, GameDirContainer, or
// GameDirEmpty using only stats and bounded directory walks (no PE
// parsing). Executable candidacy, skip tokens, and the depth cap are
// exactly findMainExe's, so the predicate never disagrees with the
// scanner about what counts as a game binary.
//
// Classification rules, in order:
//   - an exe at depth ≤ 1 (in dir or one level down) → GameDirGame;
//   - no gamey children (no immediate non-dot subdirectory yields an exe
//     under findMainExe's depth-bounded walk) → GameDirEmpty, because any
//     exe at depth 1-3 would make its top-level ancestor gamey;
//   - exactly one gamey child but dir's own exe reaches it within two
//     levels (engine-style layouts such as Binaries/Win64, where the lone
//     subdirectory exists to hold the binaries) → GameDirGame;
//   - otherwise → GameDirContainer: a library root whose games live one
//     level down (or a single child that nests its exe deeper than an
//     engine layout would).
func ClassifyGameDir(ctx context.Context, dir string) (GameDirKind, error) {
	shallow, err := findMainExeWithin(ctx, dir, 1)
	if err != nil {
		return GameDirEmpty, err
	}
	if shallow != "" {
		return GameDirGame, nil
	}
	gamey, err := gameyChildren(ctx, dir)
	if err != nil {
		return GameDirEmpty, err
	}
	if gamey == 0 {
		return GameDirEmpty, nil
	}
	if gamey == 1 {
		medium, err := findMainExeWithin(ctx, dir, 2)
		if err != nil {
			return GameDirEmpty, err
		}
		if medium != "" {
			return GameDirGame, nil
		}
	}
	return GameDirContainer, nil
}

// gameyChildren counts dir's immediate non-dot subdirectories that yield a
// main executable under findMainExe's own depth-bounded walk. Broken
// symlinks and non-directory entries are ignored.
func gameyChildren(ctx context.Context, dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	gamey := 0
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return gamey, err
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		child := filepath.Join(dir, e.Name())
		if e.IsDir() {
			// plain directory
		} else if e.Type()&fs.ModeSymlink != 0 {
			st, err := os.Stat(child)
			if err != nil || !st.IsDir() {
				continue
			}
			// Resolve the link: findMainExeWithin walks with WalkDir, which
			// does not descend a symlink root, so the unresolved path would
			// count as non-gamey while ScanRecursive (canonicalizing first)
			// scans the target.
			resolved, err := filepath.EvalSymlinks(child)
			if err != nil {
				continue
			}
			child = resolved
		} else {
			continue
		}
		exe, err := findMainExeWithin(ctx, child, maxExeDepth)
		if err != nil {
			return gamey, err
		}
		if exe != "" {
			gamey++
		}
	}
	return gamey, nil
}
