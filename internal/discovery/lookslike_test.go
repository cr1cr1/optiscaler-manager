package discovery

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The fixtures below reuse mkGameBin from recursive_test.go, which makes the
// file acceptable to the current platform's exe candidacy rules.

func TestLooksLikeGameDir_OwnExeAtRoot_IsGame(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "GameA")
	mkGameBin(t, filepath.Join(dir, "gamea"), 1<<20)

	if !LooksLikeGameDir(context.Background(), dir) {
		t.Error("exe at dir root: LooksLikeGameDir = false, want true")
	}
	assertKind(t, dir, GameDirGame)
}

func TestLooksLikeGameDir_OwnExeInImmediateSubdir_IsGame(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "GameB")
	mkGameBin(t, filepath.Join(dir, "bin", "gameb"), 1<<20)

	if !LooksLikeGameDir(context.Background(), dir) {
		t.Error("exe one level down: LooksLikeGameDir = false, want true")
	}
	assertKind(t, dir, GameDirGame)
}

func TestLooksLikeGameDir_TwoGameyChildren_IsContainer(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Games")
	mkGameBin(t, filepath.Join(dir, "GameOne", "bin", "gameone"), 1<<20)
	mkGameBin(t, filepath.Join(dir, "GameTwo", "bin", "gametwo"), 1<<20)

	if LooksLikeGameDir(context.Background(), dir) {
		t.Error("two gamey children: LooksLikeGameDir = true, want false (container)")
	}
	assertKind(t, dir, GameDirContainer)
}

func TestLooksLikeGameDir_OneGameyChildNoOwnExe_IsContainer(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Games")
	mkGameBin(t, filepath.Join(dir, "OnlyGame", "bin", "x64", "onlygame"), 1<<20)

	if LooksLikeGameDir(context.Background(), dir) {
		t.Error("one gamey child, no own exe: LooksLikeGameDir = true, want false (container)")
	}
	assertKind(t, dir, GameDirContainer)
}

func TestLooksLikeGameDir_DeepExeNoGameyChildren_IsGame(t *testing.T) {
	// Engine-style layout: the game exe hides at Binaries/Win64 and the only
	// immediate subdirectory exists to hold it.
	dir := filepath.Join(t.TempDir(), "EngineGame")
	mkGameBin(t, filepath.Join(dir, "Binaries", "Win64", "enginegame"), 1<<20)

	if !LooksLikeGameDir(context.Background(), dir) {
		t.Error("deep own exe (Binaries/Win64): LooksLikeGameDir = false, want true")
	}
	assertKind(t, dir, GameDirGame)
}

func TestLooksLikeGameDir_ExeAtDepth4_NoGameyChild_NotGame(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "TooDeep")
	mkGameBin(t, filepath.Join(dir, "a", "b", "c", "d", "toodeep"), 1<<20)

	if LooksLikeGameDir(context.Background(), dir) {
		t.Error("exe beyond depth cap: LooksLikeGameDir = true, want false")
	}
	// The exe sits 4 levels below dir, so dir itself has no exe; but from the
	// immediate child "a" the same exe is within depth 3, making "a" gamey:
	// the directory classifies as a container rather than empty.
	assertKind(t, dir, GameDirContainer)
}

func TestLooksLikeGameDir_NoExeAnywhere_NotGame(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Docs")
	writeFile(t, filepath.Join(dir, "readme.txt"), "hello")

	if LooksLikeGameDir(context.Background(), dir) {
		t.Error("no exe anywhere: LooksLikeGameDir = true, want false")
	}
	assertKind(t, dir, GameDirEmpty)
}

func TestLooksLikeGameDir_UninstallerOnlyExe_NotGame(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Leftovers")
	mkGameBin(t, filepath.Join(dir, "unins000"), 1<<20)

	if LooksLikeGameDir(context.Background(), dir) {
		t.Error("only a skip-token binary: LooksLikeGameDir = true, want false")
	}
	assertKind(t, dir, GameDirEmpty)
}

func TestLooksLikeGameDir_DotDirsAndBrokenSymlinks_Ignored(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Hidden")
	mkGameBin(t, filepath.Join(dir, ".secret", "hidden"), 1<<20)
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "broken")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	}

	if LooksLikeGameDir(context.Background(), dir) {
		t.Error("exe only inside a dot dir: LooksLikeGameDir = true, want false")
	}
	assertKind(t, dir, GameDirEmpty)
}

// TestClassifyGameDir_SymlinkedGameChild: a symlink child pointing at a real
// game dir must count as gamey — ScanRecursive canonicalizes the link and
// scans the target, so the classifier must agree the parent is a container,
// not an empty directory.
func TestClassifyGameDir_SymlinkedGameChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	target := filepath.Join(t.TempDir(), "RealGame")
	mkGameBin(t, filepath.Join(target, "bin", "realgame"), 1<<20)

	dir := filepath.Join(t.TempDir(), "Games")
	writeFile(t, filepath.Join(dir, "Notes", "readme.txt"), "hello")
	if err := os.Symlink(target, filepath.Join(dir, "LinkedGame")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	assertKind(t, dir, GameDirContainer)
}

// TestClassifyGameDir_SingleSymlinkedChild: a lone symlink child whose
// target is a game dir, with no exe of the parent's own, classifies as a
// container (the parent is not itself the game).
func TestClassifyGameDir_SingleSymlinkedChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	target := filepath.Join(t.TempDir(), "OnlyGame")
	mkGameBin(t, filepath.Join(target, "bin", "x64", "onlygame"), 1<<20)

	dir := filepath.Join(t.TempDir(), "Games")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "LinkedGame")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	assertKind(t, dir, GameDirContainer)
}

func assertKind(t *testing.T, dir string, want GameDirKind) {
	t.Helper()
	got, err := ClassifyGameDir(context.Background(), dir)
	if err != nil {
		t.Fatalf("ClassifyGameDir(%s): %v", dir, err)
	}
	if got != want {
		t.Errorf("ClassifyGameDir(%s) = %v, want %v", dir, got, want)
	}
}
