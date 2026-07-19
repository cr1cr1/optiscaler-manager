package discovery

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// mkGameBin creates a file that the current platform's recursive scanner must
// accept as a game binary: executable bit on unix, .exe suffix on Windows,
// Mach-O magic on darwin.
func mkGameBin(t *testing.T, path string, size int64) string {
	t.Helper()
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(path), ".exe") {
		path += ".exe"
	}
	if runtime.GOOS == "darwin" {
		writeFile(t, path, "\xcf\xfa\xed\xfe"+strings.Repeat("\x00", 60))
		return path
	}
	writeSized(t, path, size)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatalf("chmod %s: %v", path, err)
		}
	}
	return path
}

func TestRecursiveScan_Depth23AndExeHeuristics(t *testing.T) {
	root := t.TempDir()

	// GameAlpha: real binary at depth 2; skipped-token binaries (bigger,
	// shallower) must not win.
	alpha := filepath.Join(root, "GameAlpha")
	bad1 := mkGameBin(t, filepath.Join(alpha, "setup"), 9<<20)
	_ = bad1
	mkGameBin(t, filepath.Join(alpha, "unins000"), 9<<20)
	mkGameBin(t, filepath.Join(alpha, "bin", "crashhandler"), 9<<20)
	alphaExe := mkGameBin(t, filepath.Join(alpha, "bin", "x64", "gamealpha"), 1<<20)

	// GameBeta: only binary sits at depth 4 -> no exe resolved.
	mkGameBin(t, filepath.Join(root, "GameBeta", "a", "b", "c", "d", "gamebeta"), 1<<20)

	// GameGamma: name match at depth 2 beats a bigger non-matching binary at
	// depth 1.
	gamma := filepath.Join(root, "GameGamma")
	mkGameBin(t, filepath.Join(gamma, "zzz"), 8<<20)
	gammaExe := mkGameBin(t, filepath.Join(gamma, "sub", "gamegamma"), 1<<20)

	// GameDelta: no name matches -> larger size wins.
	delta := filepath.Join(root, "GameDelta")
	mkGameBin(t, filepath.Join(delta, "aaa"), 1<<20)
	deltaExe := mkGameBin(t, filepath.Join(delta, "bbb"), 5<<20)

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanRecursive: %v", err)
	}
	byName := map[string]string{}
	for _, g := range games {
		t.Logf("game: %+v", g)
		byName[g.Name] = g.ExePath
	}
	if got := byName["GameAlpha"]; got != alphaExe {
		t.Errorf("GameAlpha exe = %q, want %q (depth-2 real exe over shallow skip-tokens)", got, alphaExe)
	}
	if got := byName["GameBeta"]; got != "" {
		t.Errorf("GameBeta exe = %q, want \"\" (binary beyond depth 3)", got)
	}
	if got := byName["GameGamma"]; got != gammaExe {
		t.Errorf("GameGamma exe = %q, want name-match %q", got, gammaExe)
	}
	if got := byName["GameDelta"]; got != deltaExe {
		t.Errorf("GameDelta exe = %q, want larger %q", got, deltaExe)
	}
	for _, g := range games {
		if g.Store != domain.StoreManual {
			t.Errorf("game %s store = %v, want Manual", g.Name, g.Store)
		}
	}
}

func TestRecursiveScan_RescanNoDupes(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "GameOne")
	mkGameBin(t, filepath.Join(gameDir, "gameone"), 1<<20)

	// A symlinked alias of the same game dir must not produce a second entry.
	if runtime.GOOS != "windows" {
		if err := os.Symlink(gameDir, filepath.Join(root, "GameOneAlias")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	}

	first, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanRecursive: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first scan: got %d games %+v, want 1", len(first), first)
	}
	second, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanRecursive rescan: %v", err)
	}
	if len(second) != len(first) {
		t.Fatalf("rescan: got %d games, want %d (idempotent)", len(second), len(first))
	}
	if second[0].InstallDir != first[0].InstallDir || second[0].Name != first[0].Name {
		t.Fatalf("rescan mismatch: %+v vs %+v", second[0], first[0])
	}
	t.Logf("idempotent scan: %+v", first[0])
}
