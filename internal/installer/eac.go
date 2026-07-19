package installer

import (
	"os"
	"path/filepath"
)

// EACProtected reports whether the game ships an Easy Anti-Cheat launcher.
// OptiScaler injection into such games risks a ban; callers must warn first.
func EACProtected(gameRoot string) bool {
	_, err := os.Stat(filepath.Join(gameRoot, "start_protected_game.exe"))
	return err == nil
}
