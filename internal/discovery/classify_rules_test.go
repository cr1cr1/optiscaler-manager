package discovery

import (
	"context"
	"path/filepath"
	"testing"
)

// The user's "Steam" row: a Steam client install directory has steam.exe at
// its top, so the own-exe rule made it a game row even though its SteamApps
// child holds real games. A container child must outrank the own exe.
func TestClassify_OwnExeWithContainerChild_IsContainer(t *testing.T) {
	root := t.TempDir()
	steam := filepath.Join(root, "Steam")
	writePEExe(t, steam, "steam.exe", "Steam")
	writePEExe(t, steam, "SteamApps/common/GameA/GameA.exe", "Game A Title")

	assertKind(t, steam, GameDirContainer)

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, filepath.Join(steam, "SteamApps", "common", "GameA"))
}

// A Steam client dir without a SteamApps child (fresh install, games in
// another library) is still not a game: the steam.exe + Steam.dll pair
// marks a platform install, which is always a container — even with no
// game-bearing children at all.
func TestClassify_SteamClientSentinel_IsContainer(t *testing.T) {
	root := t.TempDir()
	steam := filepath.Join(root, "Steam")
	writePEExe(t, steam, "steam.exe", "Steam")
	writePEExe(t, steam, "Steam.dll", "Steam")
	writeFile(t, filepath.Join(steam, "config", "config.vdf"), "\"config\" {}")
	writeFile(t, filepath.Join(steam, "logs", "bootstrap.log"), "log")

	assertKind(t, steam, GameDirContainer)

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 0 {
		t.Errorf("steam client dir produced rows: %v", games)
	}
}

// Sentinel matching is case-insensitive (Windows-style Steam installs vary).
func TestClassify_SteamClientSentinel_CaseInsensitive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Steam")
	writePEExe(t, dir, "STEAM.EXE", "Steam")
	writePEExe(t, dir, "steam.DLL", "Steam")

	assertKind(t, dir, GameDirContainer)
}

// A game dir whose own exe sits next to a container child (a bundled
// sub-library) is a container, not a game — the own exe belongs to the
// collection. The both-case with only a GAME child still yields both rows
// (pinned by TestScanGameRootAlsoSurfacesChildGames).
func TestScan_OwnExeWithContainerChild_RootDoesNotRow(t *testing.T) {
	root := t.TempDir()
	both := filepath.Join(root, "Both")
	writePEExe(t, both, "game.exe", "Both Root Game")
	writePEExe(t, both, "Collection/InnerGame/inner.exe", "Inner Game Title")

	assertKind(t, both, GameDirContainer)

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, filepath.Join(both, "Collection", "InnerGame"))
}

// A game dir with its own exe and only game children keeps the both-case
// when it is the scan root: root row plus child rows.
func TestScan_OwnExeWithGameChild_StillBothRows(t *testing.T) {
	root := t.TempDir()
	both := filepath.Join(root, "Both")
	writePEExe(t, both, "game.exe", "Both Root Game")
	writePEExe(t, both, "sub/sub.exe", "Sub Game")

	assertKind(t, both, GameDirGame)

	rows := scanRows(t, both)
	assertOnlyRows(t, rows, both, filepath.Join(both, "sub"))
}

// Engine-named directories added directly as scan roots (a Proton folder,
// a compatdata tree) must not spray tooling rows: the root guard refuses
// them at the door.
func TestScanRecursive_EngineNamedRootYieldsNothing(t *testing.T) {
	root := t.TempDir()
	proton := filepath.Join(root, "Proton - Experimental")
	writePEExe(t, proton, "files/bin/wine.exe", "Wine")

	games, err := ScanRecursive(context.Background(), proton)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 0 {
		t.Errorf("proton root produced rows: %v", games)
	}
	assertKind(t, proton, GameDirEmpty)
}
