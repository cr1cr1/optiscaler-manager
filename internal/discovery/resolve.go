package discovery

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/cr1cr1/optiscaler-manager/internal/classify"
)

// skipExeNames are substring tokens (case-insensitive) of executables that are
// never the game binary: crash handlers, installers, launchers, redists.
var skipExeNames = []string{
	"crash", "redist", "setup", "installer", "launcher",
	"unrealcefsubprocess", "prerequisites", "unins",
}

// ResolveInstallDir returns the directory inside gameRoot where injection
// DLLs must be placed: the directory of the game's real executable.
//
// Rule order: the UE5 "Phoenix/Binaries/Win64" layout wins outright;
// otherwise every candidate .exe is scored (+15 name similarity to the game
// folder, +5 under a Binaries/Win64 segment, +10 when larger than 5 MiB,
// +25 when upscaler DLLs sit beside it) and the best directory wins.
// Ties break to the lexicographically smallest path.
func ResolveInstallDir(gameRoot string) (string, error) {
	if ue5 := ue5Win64Dir(gameRoot); ue5 != "" {
		return ue5, nil
	}

	gameName := squeezeName(filepath.Base(gameRoot))
	bestScore := -1
	bestDir := ""
	found := false

	err := filepath.WalkDir(gameRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != gameRoot {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".exe") {
			return nil
		}
		if skipExe(d.Name()) {
			return nil
		}

		score := 0
		stem := squeezeName(strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())))
		if stem == gameName || strings.Contains(stem, gameName) || strings.Contains(gameName, stem) {
			score += 15
		}
		if hasWin64Segment(path) {
			score += 5
		}
		if st, err := os.Stat(path); err == nil && st.Size() > 5<<20 {
			score += 10
		}
		if len(classify.Dir(filepath.Dir(path))) > 0 {
			score += 25
		}

		dir := filepath.Dir(path)
		if !found || score > bestScore || (score == bestScore && path < bestDir) {
			found = true
			bestScore = score
			bestDir = dir
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("scan %s: %w", gameRoot, err)
	}
	if !found {
		return "", fmt.Errorf("no game executable found under %s", gameRoot)
	}
	return bestDir, nil
}

// ue5Win64Dir returns the UE5 shipping-binary directory when present.
func ue5Win64Dir(gameRoot string) string {
	dir := filepath.Join(gameRoot, "Phoenix", "Binaries", "Win64")
	if exeIn(dir) {
		return dir
	}
	return ""
}

func exeIn(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".exe") {
			return true
		}
	}
	return false
}

// squeezeName lowercases s and drops separator characters so exe stems and
// folder names compare the way users read them ("FarCry5" == "Far Cry 5").
func squeezeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '.' || r == ' ' {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}

func skipExe(base string) bool {
	lower := strings.ToLower(base)
	for _, tok := range skipExeNames {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

// hasWin64Segment reports whether path passes through a Binaries/Win64 pair.
func hasWin64Segment(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i := 0; i+1 < len(parts); i++ {
		if strings.EqualFold(parts[i], "Binaries") && strings.EqualFold(parts[i+1], "Win64") {
			return true
		}
	}
	return false
}
