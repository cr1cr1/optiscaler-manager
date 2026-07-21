package discovery

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
)

// GameTitle resolves a game's display title by reliability: the PE
// StringFileInfo title of the main executable first (ProductName, then
// FileDescription), then the exe's filename stem when it carries more
// information than the folder, then the folder name. Unreadable or
// title-less executables move down the chain.
func GameTitle(exe, folder string) string {
	if exe == "" {
		return folder
	}
	if title := pever.TitleFromFile(exe); title != "" {
		return title
	}
	return exeStemTitle(exe, folder)
}

// platformStemTokens are trailing exe-name tokens that describe the build,
// not the game ("TempestRising-Win64-Shipping" → "TempestRising").
var platformStemTokens = map[string]bool{
	"win64": true, "win32": true, "x64": true, "x86": true, "x86_64": true,
	"amd64": true, "dx9": true, "dx10": true, "dx11": true, "dx12": true,
	"vk": true, "vulkan": true, "shipping": true, "master": true,
	"release": true, "profile": true, "final": true, "retail": true,
	"rwdi": true, "wingdk": true, "win_gdk": true, "msstore": true,
}

// genericStemNames are exe stems that carry no title information.
var genericStemNames = map[string]bool{
	"game": true, "main": true, "app": true, "start": true,
	"client": true, "play": true, "run": true, "elevate": true,
}

// exeStemTitle derives a title from the exe's filename stem when it is more
// informative than the folder name; otherwise it returns the folder.
func exeStemTitle(exe, folder string) string {
	base := filepath.Base(exe)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	// Strip trailing platform tokens iteratively: "game-win64-shipping".
	for {
		i := strings.LastIndexAny(stem, "-_. ")
		if i < 0 || !platformStemTokens[strings.ToLower(stem[i+1:])] {
			break
		}
		stem = stem[:i]
	}
	lower := strings.ToLower(stem)
	if stem == "" || len(stem) < 3 || platformStemTokens[lower] ||
		genericStemNames[lower] || skippedBinaryName(stem) || allDigitOrSep(stem) {
		return folder
	}
	norm := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '-' || r == '_' || r == '.' || r == ' ' {
				return -1
			}
			return unicode.ToLower(r)
		}, s)
	}
	fn, sn := norm(folder), norm(stem)
	if fn == sn || strings.Contains(fn, sn) || strings.Contains(sn, fn) {
		return folder // the stem only echoes the folder; the folder reads better
	}
	return prettifyStem(stem)
}

// allDigitOrSep reports whether s holds only digits and separators.
func allDigitOrSep(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && c != '-' && c != '_' && c != '.' && c != ' ' {
			return false
		}
	}
	return len(s) > 0
}

// prettifyStem turns an exe stem into a display title: separators become
// spaces, camel humps split, runs of whitespace collapse.
func prettifyStem(stem string) string {
	spaced := strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '.' {
			return ' '
		}
		return r
	}, stem)
	var b strings.Builder
	prev := rune(0)
	for i, r := range spaced {
		next, _ := utf8.DecodeRuneInString(spaced[i+len(string(r)):])
		if i > 0 && r != ' ' && prev != ' ' &&
			unicode.IsUpper(r) && (unicode.IsLower(prev) ||
			(unicode.IsUpper(prev) && unicode.IsLower(next))) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		prev = r
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// recursiveSkipTokens are substring tokens (case-insensitive) of binaries
// that are never the game executable: uninstallers, installers, crash
// handlers, updaters, helper services, and engine subprocess hosts.
var recursiveSkipTokens = []string{
	"unins", "setup", "install", "redist", "vcredist", "dxsetup",
	"crash", "handler", "launcher", "updater", "patcher", "helper",
	"service", "report", "benchmark", "unrealcefsubprocess", "prerequisites",
}

// maxExeDepth is how many directory levels below a game directory the
// recursive scan descends looking for the main executable. Four levels
// cover the deepest real layout seen in the wild (Prey ships
// Binaries/Danielle/x64-Epic/Release/Prey.exe).
const maxExeDepth = 4

// ScanRecursive resolves the games under root. When root itself is a game
// (yields a main executable) it gets its own row; either way its children
// are then evaluated with the same game/container/empty rules as
// AddDirectory (see ClassifyGameDir): children that are games produce rows
// (engine folders like bin/Binaries/Win64 never do — they hold the parent's
// own binaries), containers are recursed into transparently (never rows),
// and exe-less children are skipped. Candidates named like
// uninstallers/installers/crash handlers are skipped; ranking prefers a name
// match to the game folder, then larger size, then 64-bit-looking names.
// Games are deduplicated by canonical install directory, so rescanning is
// idempotent.
// ScanRecursive resolves the games under root with the default
// identification chain (see ScanRecursiveWithResolver).
func ScanRecursive(ctx context.Context, root string) ([]domain.Game, error) {
	return ScanRecursiveWithResolver(ctx, root, ChainResolver(nil))
}

// ScanRecursiveWithResolver is ScanRecursive with a caller-chosen title
// resolver (v0.8: the session injects its settings-aware chain).
func ScanRecursiveWithResolver(ctx context.Context, root string, res TitleResolver) ([]domain.Game, error) {
	root = canonicalPath(root)
	if engineFolderName(filepath.Base(root)) {
		// Plumbing added directly (a Proton folder, a compatdata tree)
		// holds no games of its own; refuse it like an empty directory.
		log.Debug().Str("dir", root).Msg("recursive scan: engine-named root, nothing to scan")
		return nil, nil
	}
	kind, err := ClassifyGameDir(ctx, root)
	if err != nil {
		return nil, err
	}
	var games []domain.Game
	if kind == GameDirGame {
		exe, err := findMainExe(ctx, root)
		if err != nil {
			return nil, err
		}
		title := res(root, exe)
		games = append(games, domain.Game{
			AppID:       "manual_" + filepath.Base(root),
			Name:        title.Name,
			InstallDir:  root,
			Store:       domain.StoreManual,
			ExePath:     exe,
			SteamAppID:  title.SteamAppID,
			TitleSource: title.Source,
			AppName:     title.EpicAppName,
		})
	}
	sub, err := scanLevel(ctx, root, 0, map[string]bool{root: true}, res)
	if err != nil {
		return games, err
	}
	return append(games, sub...), nil
}

// maxContainerDepth bounds how many nested container levels a scan recurses
// through (Games → Steam → common → …). Deeper nesting is unusual enough
// that logging and stopping beats walking arbitrarily deep trees.
const maxContainerDepth = 4

// scanLevel evaluates the immediate children of dir: games become rows,
// containers are recursed into, anything else is skipped.
func scanLevel(ctx context.Context, dir string, depth int, seen map[string]bool, res TitleResolver) ([]domain.Game, error) {
	if depth > maxContainerDepth {
		log.Debug().Str("dir", dir).Msg("recursive scan: container nesting limit reached")
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var games []domain.Game
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return games, err
		}
		if e.IsDir() {
			// plain directory
		} else if e.Type()&fs.ModeSymlink != 0 {
			st, err := os.Stat(filepath.Join(dir, e.Name()))
			if err != nil || !st.IsDir() {
				continue
			}
		} else {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if engineFolderName(e.Name()) {
			log.Debug().Str("dir", filepath.Join(dir, e.Name())).Msg("recursive scan: engine folder, not a game row")
			continue
		}
		child := canonicalPath(filepath.Join(dir, e.Name()))
		if seen[child] {
			continue
		}
		seen[child] = true

		kind, err := ClassifyGameDir(ctx, child)
		if err != nil {
			if ctx.Err() != nil {
				return games, err
			}
			log.Debug().Err(err).Str("dir", child).Msg("recursive scan: classification failed, skipping")
			continue
		}
		switch kind {
		case GameDirGame:
			exe, err := findMainExe(ctx, child)
			if err != nil {
				return games, err
			}
			if exe == "" {
				continue
			}
			title := res(child, exe)
			games = append(games, domain.Game{
				AppID:       "manual_" + e.Name(),
				Name:        title.Name,
				InstallDir:  child,
				Store:       domain.StoreManual,
				ExePath:     exe,
				SteamAppID:  title.SteamAppID,
				TitleSource: title.Source,
				AppName:     title.EpicAppName,
			})
		case GameDirContainer:
			sub, err := scanLevel(ctx, child, depth+1, seen, res)
			if err != nil {
				return games, err
			}
			games = append(games, sub...)
		default:
			log.Debug().Str("dir", child).Msg("recursive scan: no game here, skipping")
		}
	}
	return games, nil
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
			if plumbingWalkDir(d.Name(), filepath.Base(filepath.Dir(path))) {
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
// directly inside root is depth 0 when path is that file's parent dir;
// depth 1 is an immediate subdirectory; "a/b/c" is depth 3).
func depthOf(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(rel, string(filepath.Separator)))
}

// scoreCandidate builds the ranked record for one accepted binary: name
// similarity to the game folder dominates (compared separator-insensitively,
// so "FarCry5.exe" matches "Far Cry 5"), then file size, then a
// 64-bit-looking name.
func scoreCandidate(path, base, folder string, size int64) *exeCandidate {
	squeeze := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r == '-' || r == '_' || r == '.' || r == ' ' {
				return -1
			}
			return unicode.ToLower(r)
		}, s)
	}
	stem := squeeze(strings.TrimSuffix(base, filepath.Ext(base)))
	folder = squeeze(folder)
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
