package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// writePEExe writes a synthetic PE with the given ProductName at dir/rel.
func writePEExe(t *testing.T, dir, rel, productName string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	pe := testutil.StringInfoPE(false, map[string]string{"ProductName": productName}, [4]uint16{1, 0, 0, 0})
	if err := os.WriteFile(p, pe, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestScanNestedContainerSurfacesGamesNotContainer is the user's reported
// failure: a container ("Steam") nested in the scan root must NOT become a
// row itself, and the games inside it must each surface with their own PE
// titles — today the container steals one game's title and the rest are lost.
func TestScanNestedContainerSurfacesGamesNotContainer(t *testing.T) {
	root := t.TempDir()
	writePEExe(t, root, "Steam/GameA/bin/GameA.exe", "Game A Real Title")
	writePEExe(t, root, "Steam/GameB/GameB.exe", "Game B Real Title")
	writePEExe(t, root, "Direct/GameC.exe", "Game C Real Title")

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	for _, g := range games {
		rows[g.Name] = g.InstallDir
	}
	for _, title := range []string{"Game A Real Title", "Game B Real Title", "Game C Real Title"} {
		if _, ok := rows[title]; !ok {
			t.Errorf("missing row for %q; rows: %v", title, rows)
		}
	}
	for _, g := range games {
		base := filepath.Base(g.InstallDir)
		if base == "Steam" || base == "Games" {
			t.Errorf("container row present: name=%q installDir=%q", g.Name, g.InstallDir)
		}
	}
	if len(games) != 3 {
		t.Errorf("got %d rows, want exactly 3 (GameA, GameB, GameC): %v", len(games), rows)
	}
}

// TestScanDeeplyNestedContainers recurse through several container levels
// (Games → Steam → common → games) without rowing any intermediate dir.
func TestScanDeeplyNestedContainers(t *testing.T) {
	root := t.TempDir()
	writePEExe(t, root, "Games/Steam/common/GameA/GameA.exe", "Nested Game A")
	writePEExe(t, root, "Games/Steam/common/GameB/bin/GameB.exe", "Nested Game B")

	games, err := ScanRecursive(context.Background(), filepath.Join(root, "Games"))
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 {
		t.Fatalf("got %d rows, want 2: %v", len(games), games)
	}
	for _, g := range games {
		if g.Name != "Nested Game A" && g.Name != "Nested Game B" {
			t.Errorf("unexpected row: name=%q installDir=%q", g.Name, g.InstallDir)
		}
	}
}

// TestScanGameRootYieldsOnlyItself: when the scan root IS a game (own exe),
// its engine subfolders (Binaries, bin) must NOT become rows — the root is
// the game, nothing inside it is a separate game.
func TestScanGameRootYieldsOnlyItself(t *testing.T) {
	root := t.TempDir()
	writePEExe(t, root, "UnrealGame/Binaries/Win64/UEGame.exe", "UE Game")

	games, err := ScanRecursive(context.Background(), filepath.Join(root, "UnrealGame"))
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d rows, want exactly 1: %v", len(games), games)
	}
	g := games[0]
	if g.Name != "UE Game" {
		t.Errorf("name = %q, want %q (PE title)", g.Name, "UE Game")
	}
	if g.InstallDir != canonicalPath(filepath.Join(root, "UnrealGame")) {
		t.Errorf("InstallDir = %q, want the game root itself", g.InstallDir)
	}
}

// TestScanGameRootAlsoSurfacesChildGames: a directory that is both a game
// (exe at top level) and a container (a real game in a subdirectory) yields
// its own row AND the child game's row.
func TestScanGameRootAlsoSurfacesChildGames(t *testing.T) {
	root := t.TempDir()
	writePEExe(t, root, "Both/game.exe", "Both Root Game")
	writePEExe(t, root, "Both/sub/sub.exe", "Sub Game")

	games, err := ScanRecursive(context.Background(), filepath.Join(root, "Both"))
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 {
		t.Fatalf("got %d rows, want 2 (root + sub): %v", len(games), games)
	}
	names := map[string]bool{}
	for _, g := range games {
		names[g.Name] = true
	}
	for _, want := range []string{"Both Root Game", "Sub Game"} {
		if !names[want] {
			t.Errorf("missing row %q in %v", want, names)
		}
	}
}

// TestClassifyGameDir_SymlinkFanOutTerminates: classification must not
// re-evaluate directories reached through multiple links — a symlinked
// fan-out tree terminates quickly and classifies sanely.
func TestClassifyGameDir_SymlinkFanOutTerminates(t *testing.T) {
	root := t.TempDir()
	writePEExe(t, root, "real/game/game.exe", "FanOut Game")
	base := filepath.Join(root, "real")
	// Many links back into the tree: without a visited set these multiply
	// classification work exponentially.
	for i := 0; i < 30; i++ {
		if err := os.Symlink(base, filepath.Join(root, "real", "game", "loop"+string(rune('a'+i%26))+string(rune('a'+i/26)))); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "self")); err != nil {
		t.Fatal(err)
	}
	root2 := filepath.Join(root, "real")
	for i := 0; i < 30; i++ {
		target := root2
		if i%2 == 1 {
			target = base
		}
		if err := os.Symlink(target, filepath.Join(root, "real", "game", "zz"+string(rune('A'+i%26))+string(rune('A'+i/26)))); err != nil {
			t.Fatal(err)
		}
	}

	kind, err := ClassifyGameDir(context.Background(), filepath.Join(root, "real"))
	if err != nil {
		t.Fatal(err)
	}
	if kind != GameDirContainer {
		t.Errorf("kind = %v, want container (it holds the game dir)", kind)
	}
	kind, err = ClassifyGameDir(context.Background(), filepath.Join(root, "real", "game"))
	if err != nil {
		t.Fatal(err)
	}
	if kind != GameDirGame {
		t.Errorf("game dir kind = %v, want game", kind)
	}
}

// TestClassifyGameDir_PureSymlinkLoopTerminates: a tree whose only content
// is symlinks back into itself terminates immediately and is empty.
func TestClassifyGameDir_PureSymlinkLoopTerminates(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "loop"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if err := os.Symlink(root, filepath.Join(root, "loop", "back"+string(rune('a'+i%26))+string(rune('a'+i/26)))); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "loop"), filepath.Join(root, "loop", "self")); err != nil {
		t.Fatal(err)
	}

	kind, err := ClassifyGameDir(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if kind != GameDirEmpty {
		t.Errorf("kind = %v, want empty (nothing executable anywhere)", kind)
	}
}
