package discovery

import (
	"context"
	"path/filepath"
	"testing"
)

// A steamapps dir with no installed games but a partial download carrying
// an MZ exe must not row: downloading is plumbing, never game material.
func TestScan_SteamappsWithPartialDownloadDoesNotRow(t *testing.T) {
	root := t.TempDir()
	apps := filepath.Join(root, "steamapps")
	writePEExe(t, apps, "downloading/456/partialgame.exe", "Partial Game")
	writePEExe(t, apps, "compatdata/123/pfx/drive_c/windows/system32/dxdiag.exe", "DirectX")

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 0 {
		t.Errorf("steamapps with only plumbing produced rows: %v", games)
	}
	assertKind(t, apps, GameDirEmpty)
}

// A bare Wine prefix (no game installed) must not row: drive_c/windows and
// drive_c/users hold the OS, not games.
func TestScan_BareWinePrefixDoesNotRow(t *testing.T) {
	root := t.TempDir()
	pfx := filepath.Join(root, "MyPrefix", "drive_c")
	writePEExe(t, pfx, "windows/notepad.exe", "Microsoft Windows")
	writePEExe(t, pfx, "windows/system32/dxdiag.exe", "DirectX Diagnostic Tool")
	writeFile(t, filepath.Join(pfx, "users", "cri", "settings.ini"), "ini")

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 0 {
		t.Errorf("bare wine prefix produced rows: %v", games)
	}
	assertKind(t, filepath.Join(root, "MyPrefix"), GameDirEmpty)
}

// A real game installed inside a Lutris prefix (drive_c/GOG Games/...) must
// still surface at the prefix root — only the windows/users system subtrees
// are pruned.
func TestScan_LutrisGameUnderDriveCStillRows(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "MyGame")
	writePEExe(t, game, "drive_c/GOG Games/MyGame/game.exe", "My Real Game")
	writePEExe(t, game, "drive_c/windows/system32/whoami.exe", "whoami")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, game)
	if rows[canonicalPath(game)] != "My Real Game" {
		t.Errorf("title = %q, want the PE title", rows[canonicalPath(game)])
	}
}

// Prey ships its exe four levels down under the engine-named Binaries
// folder (Binaries/Danielle/x64-Epic/Release/Prey.exe): the game must row
// at its root, not vanish.
func TestScan_PreyDeepEnginePathRows(t *testing.T) {
	root := t.TempDir()
	prey := filepath.Join(root, "PREY")
	writePEExe(t, prey, "Binaries/Danielle/x64-Epic/Release/Prey.exe", "Prey")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, prey)
	if rows[canonicalPath(prey)] != "Prey" {
		t.Errorf("title = %q, want the PE title", rows[canonicalPath(prey)])
	}
}

// Days Gone keeps its row at BendGame and CRS stays dead at the deeper
// depth budget (the engine-named Engine subtree is never consulted).
func TestScan_DaysGoneStillRowsBendGameOnlyAtDepth4(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Days Gone Broken Road")
	writePEExe(t, game, "BendGame/Binaries/Win64/DaysGone.exe", "Days Gone")
	writePEExe(t, game, "Engine/Binaries/ThirdParty/CRS/deep/deeper/crs.exe", "CRS")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, filepath.Join(game, "BendGame"))
}

// ResolveInstallDir must prefer the name-matched game exe over a larger
// redist installer even when the game folder name contains spaces
// ("FarCry5.exe" is the main exe of "Far Cry 5").
func TestResolveInstallDir_SpaceyNamePrefersGameExe(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Far Cry 5")
	writeSized(t, filepath.Join(root, "bin", "FarCry5.exe"), 1<<20)
	writeSized(t, filepath.Join(root, "Support", "Software", "MicrosoftNetFramework 4.6.1", "NDP461-KB3102436-x86-x64-AllOS-ENU.exe"), 9<<20)

	got, err := ResolveInstallDir(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "bin")
	if got != want {
		t.Errorf("ResolveInstallDir = %q, want %q (not the .NET installer dir)", got, want)
	}
}
