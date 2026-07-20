package discovery

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
)

// maxExeTitleRead caps the bounded read used for PE title extraction;
// executables larger than this keep their folder name.
const maxExeTitleRead = 128 << 20

// recursiveSkipTokens are substring tokens (case-insensitive) of binaries
// that are never the game executable: uninstallers, installers, crash
// handlers, updaters, and helper services.
var recursiveSkipTokens = []string{
	"unins", "setup", "install", "redist", "vcredist", "dxsetup",
	"crash", "handler", "launcher", "updater", "patcher", "helper",
	"service", "report", "benchmark",
}

// maxExeDepth is how many directory levels below a game directory the
// recursive scan descends looking for the main executable.
const maxExeDepth = 3

// ScanRecursive treats each subdirectory of root as one installed game and
// resolves its main executable by descending at most maxExeDepth levels.
// Candidates named like uninstallers/installers/crash handlers are skipped;
// ranking prefers a name match to the game folder, then larger size, then
// 64-bit-looking names. Games are deduplicated by canonical install
// directory, so rescanning a root is idempotent.
func ScanRecursive(ctx context.Context, root string) ([]domain.Game, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var games []domain.Game
	seen := map[string]bool{}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return games, err
		}
		if e.IsDir() {
			// plain directory
		} else if e.Type()&fs.ModeSymlink != 0 {
			st, err := os.Stat(filepath.Join(root, e.Name()))
			if err != nil || !st.IsDir() {
				continue
			}
		} else {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := canonicalPath(filepath.Join(root, e.Name()))
		if seen[dir] {
			continue
		}
		seen[dir] = true

		exe, err := findMainExe(ctx, dir)
		if err != nil {
			return games, err
		}
		if exe == "" {
			log.Debug().Str("dir", dir).Msg("recursive scan: no game executable found")
		}
		games = append(games, domain.Game{
			AppID:      "manual_" + e.Name(),
			Name:       gameTitle(exe, e.Name()),
			InstallDir: dir,
			Store:      domain.StoreManual,
			ExePath:    exe,
		})
	}
	return games, nil
}

// gameTitle prefers the PE StringFileInfo title of the main executable over
// the folder name; unreadable or title-less executables keep the folder
// name.
func gameTitle(exe, folder string) string {
	if exe == "" {
		return folder
	}
	data, err := pever.ReadBounded(exe, maxExeTitleRead)
	if err != nil {
		log.Debug().Err(err).Str("exe", exe).Msg("recursive scan: exe unreadable for title")
		return folder
	}
	if title := pever.ExtractTitle(data); title != "" {
		return title
	}
	return folder
}

type exeCandidate struct {
	path      string
	nameScore int
	size      int64
	is64      bool
}

// FindMainExe resolves the best game executable candidate under gameDir
// (depth ≤ maxExeDepth), or "" when none qualifies. It is the single-game
// form of the recursive scanner's exe picking.
func FindMainExe(ctx context.Context, gameDir string) (string, error) {
	return findMainExe(ctx, gameDir)
}

// findMainExe walks gameDir (depth ≤ maxExeDepth) and returns the best game
// executable candidate, or "" when none qualifies.
func findMainExe(ctx context.Context, gameDir string) (string, error) {
	return findMainExeWithin(ctx, gameDir, maxExeDepth)
}

// findMainExeWithin is findMainExe with a caller-chosen depth cap, so the
// game-dir predicate can ask shallower questions with identical candidacy,
// skip-token, and ranking rules.
func findMainExeWithin(ctx context.Context, gameDir string, maxDepth int) (string, error) {
	folder := strings.ToLower(filepath.Base(gameDir))
	var best *exeCandidate
	err := filepath.WalkDir(gameDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if path == gameDir {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if depthOf(gameDir, path) > maxDepth {
				return filepath.SkipDir
			}
			if ok, skip := dirCandidate(path); ok {
				consider(&best, scoreCandidate(path, d.Name(), folder, 0))
				if skip {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		ok, size := fileCandidate(path, d)
		if !ok {
			return nil
		}
		consider(&best, scoreCandidate(path, d.Name(), folder, size))
		return nil
	})
	if err != nil {
		return "", err
	}
	if best == nil {
		return "", nil
	}
	return best.path, nil
}

// depthOf returns how many directory levels path sits below root (a file
// directly inside root is depth 0 when path is that file's parent dir; a
// subdirectory "a/b/c" is depth 3).
func depthOf(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(rel, string(filepath.Separator)))
}

// scoreCandidate builds the ranked record for one accepted binary: name
// similarity to the game folder dominates, then file size, then a
// 64-bit-looking name.
func scoreCandidate(path, base, folder string, size int64) *exeCandidate {
	stem := strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
	c := &exeCandidate{path: path, size: size}
	switch {
	case stem == folder:
		c.nameScore = 2
	case strings.Contains(stem, folder) || strings.Contains(folder, stem):
		c.nameScore = 1
	}
	lower := strings.ToLower(base)
	if strings.Contains(lower, "x64") || strings.Contains(lower, "win64") ||
		strings.Contains(lower, "_64") || strings.Contains(lower, "64bit") {
		c.is64 = true
	}
	return c
}

// consider keeps the better of the incumbent and the challenger: name score,
// then size, then 64-bit, then the lexicographically smaller path for a
// fully deterministic result.
func consider(best **exeCandidate, c *exeCandidate) {
	b := *best
	if b == nil ||
		c.nameScore > b.nameScore ||
		(c.nameScore == b.nameScore && c.size > b.size) ||
		(c.nameScore == b.nameScore && c.size == b.size && c.is64 && !b.is64) ||
		(c.nameScore == b.nameScore && c.size == b.size && c.is64 == b.is64 && c.path < b.path) {
		*best = c
	}
}

// skippedBinaryName reports whether base names an installer/uninstaller/
// helper style binary that is never the game executable.
func skippedBinaryName(base string) bool {
	lower := strings.ToLower(base)
	for _, tok := range recursiveSkipTokens {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

// canonicalPath returns p as an absolute, symlink-resolved, cleaned path so
// games can be deduplicated across aliases and stores.
func canonicalPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Clean(p)
}
