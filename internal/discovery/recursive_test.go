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

// --- synthetic PE fixture (mirrors internal/pever/pe_test.go) -------------

func utf16leFixture(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range s {
		if r > 0xFFFF {
			r = 0xFFFD
		}
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

func stringInfoFixture(key, value string) []byte {
	kb := utf16leFixture(key)
	vb := utf16leFixture(value)
	valWords := len(vb)/2 + 1
	valOff := (6 + len(kb) + 2 + 3) &^ 3
	structLen := valOff + len(vb) + 2
	b := make([]byte, structLen)
	b[0], b[1] = byte(structLen), byte(structLen>>8)
	b[2], b[3] = byte(valWords), byte(valWords>>8)
	b[4], b[5] = 1, 0 // wType = text
	copy(b[6:], kb)
	copy(b[valOff:], vb)
	return b
}

// peWithProductName builds a minimal PE32+ image whose StringFileInfo
// carries the given ProductName.
func peWithProductName(name string) []byte {
	resData := stringInfoFixture("ProductName", name)
	const (
		eLfanew    = 0x40
		sectVA     = 0x1000
		sectRawOff = 0x200
		optSize    = 0xF0
	)
	b := make([]byte, sectRawOff+len(resData))
	b[0], b[1] = 'M', 'Z'
	put32 := func(off int, v uint32) {
		b[off], b[off+1], b[off+2], b[off+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
	}
	b[0x3C] = eLfanew
	copy(b[eLfanew:], "PE\x00\x00")
	coff := eLfanew + 4
	b[coff+2], b[coff+3] = 1, 0 // NumberOfSections
	b[coff+16], b[coff+17] = optSize, 0
	opt := coff + 20
	b[opt], b[opt+1] = 0x0B, 0x02 // PE32+ magic
	put32(opt+112+2*8, sectVA)    // resource data-directory entry
	put32(opt+112+2*8+4, uint32(len(resData)))
	sec := opt + optSize
	copy(b[sec:], ".rsrc\x00\x00\x00")
	put32(sec+8, uint32(len(resData)))
	put32(sec+12, sectVA)
	put32(sec+16, uint32(len(resData)))
	put32(sec+20, sectRawOff)
	copy(b[sectRawOff:], resData)
	return b
}

// --- title extraction & exe eligibility -----------------------------------

func TestScanRecursive_TitleFromPEVersionInfo(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "SomeFolder")
	writeFile(t, filepath.Join(gameDir, "game.exe"), string(peWithProductName("My Cool Game")))
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(gameDir, "game.exe"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanRecursive: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games, want 1: %+v", len(games), games)
	}
	if games[0].Name != "My Cool Game" {
		t.Errorf("Name = %q, want PE ProductName %q", games[0].Name, "My Cool Game")
	}
	if games[0].ExePath == "" {
		t.Error("ExePath empty, want the synthetic exe")
	}
}

func TestScanRecursive_FolderTitleFallback(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "PlainGame")
	mkGameBin(t, filepath.Join(gameDir, "game.exe"), 1<<10)

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanRecursive: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games, want 1: %+v", len(games), games)
	}
	if games[0].Name != "PlainGame" {
		t.Errorf("Name = %q, want folder name %q", games[0].Name, "PlainGame")
	}
}

func TestScanRecursive_AcceptsExeWithoutExecBit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("unix exe-bit relaxation is asserted on linux")
	}
	root := t.TempDir()
	gameDir := filepath.Join(root, "NoExecBit")
	writeFile(t, filepath.Join(gameDir, "game.exe"), "GAME") // 0644, no exec bit

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanRecursive: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games, want 1: %+v", len(games), games)
	}
	want := filepath.Join(gameDir, "game.exe")
	if games[0].ExePath != want {
		t.Errorf("ExePath = %q, want %q (0644 .exe ranked as candidate)", games[0].ExePath, want)
	}
}

func TestScanRecursive_SkipsSubdirWithoutExe(t *testing.T) {
	root := t.TempDir()

	// Steam: a container-style subdirectory with no exe within the depth
	// budget must not produce a row at all.
	writeFile(t, filepath.Join(root, "Steam", "readme.txt"), "not a game")
	// RealGame: a real game next to it must still be found.
	realExe := mkGameBin(t, filepath.Join(root, "RealGame", "realgame"), 1<<20)

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanRecursive: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games, want 1 (exe-less subdir skipped): %+v", len(games), games)
	}
	if games[0].Name != "RealGame" || games[0].ExePath != realExe {
		t.Errorf("game = %+v, want RealGame with exe %q", games[0], realExe)
	}
}

func TestScanRecursive_DeepExeGameStillFound(t *testing.T) {
	root := t.TempDir()

	// Engine-style layout: the exe sits at Binaries/Win64 (depth 2), which
	// is inside the depth budget, so the game must still produce a row.
	engineExe := mkGameBin(t, filepath.Join(root, "EngineGame", "Binaries", "Win64", "enginegame"), 1<<20)

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanRecursive: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games, want 1: %+v", len(games), games)
	}
	if games[0].ExePath != engineExe {
		t.Errorf("ExePath = %q, want deep exe %q", games[0].ExePath, engineExe)
	}
}

func TestScanRecursive_InstallerOnlySubdirSkipped(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "InstGame")
	for _, name := range []string{"unins000.exe", "setup.exe"} {
		writeFile(t, filepath.Join(gameDir, name), "GAME")
		if runtime.GOOS != "windows" {
			if err := os.Chmod(filepath.Join(gameDir, name), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanRecursive: %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("got %d games, want 0 (installer-only subdir yields no row): %+v", len(games), games)
	}
}

// Pin: a PE whose ProductName is a vendor TODO placeholder carries no
// usable title, so the game keeps its folder name.
func TestScanRecursive_TODOPlaceholderTitleFallsBackToFolder(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "SomeFolder")
	writeFile(t, filepath.Join(gameDir, "game.exe"), string(peWithProductName("TODO: <Product Name>")))
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(gameDir, "game.exe"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanRecursive: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games, want 1: %+v", len(games), games)
	}
	if games[0].Name != "SomeFolder" {
		t.Errorf("Name = %q, want folder fallback %q for TODO placeholder", games[0].Name, "SomeFolder")
	}
}
