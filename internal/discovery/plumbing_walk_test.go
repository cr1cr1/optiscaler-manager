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
// depth budget: the container-via-BendGame precedence and the engine
// skip protect it even with CRS at the exact depth boundary.
func TestScan_DaysGoneStillRowsBendGameOnlyAtDepth4(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Days Gone Broken Road")
	writePEExe(t, game, "BendGame/Binaries/Win64/DaysGone.exe", "Days Gone")
	writePEExe(t, game, "Engine/Binaries/ThirdParty/CRS/crs.exe", "CRS")

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

// A steamapps/common holding only Proton tooling is not a game: Proton and
// Steam Linux Runtime subtrees are platform runtime, never a game's own
// binary — the exe walk must not descend them.
func TestScan_CommonWithOnlyProtonDoesNotRow(t *testing.T) {
	root := t.TempDir()
	common := filepath.Join(root, "common")
	writePEExe(t, common, "Proton - Experimental/files/bin/wine.exe", "Wine")
	writePEExe(t, common, "SteamLinuxRuntime_sniper/files/bin/pressure-vessel.exe", "PV")

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 0 {
		t.Errorf("proton-only common produced rows: %v", games)
	}
	assertKind(t, common, GameDirEmpty)
}

// Steam workshop content (mods) is not standalone game material: it must
// not row even alongside real installed games.
func TestScan_SteamWorkshopDoesNotRow(t *testing.T) {
	root := t.TempDir()
	apps := filepath.Join(root, "steamapps")
	writePEExe(t, apps, "common/RealGame/RealGame.exe", "Real Game Title")
	writePEExe(t, apps, "workshop/content/12345/modtool.exe", "Mod Tool")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, filepath.Join(apps, "common", "RealGame"))
}

// Boundary at the new depth budget: engine-support tooling exactly four
// levels down (Engine/Binaries/ThirdParty/CRS) must stay unreachable.
func TestScan_EngineThirdPartyAtDepthBoundaryDoesNotRow(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "SomeFolder")
	writePEExe(t, folder, "Engine/Binaries/ThirdParty/CRS/crs.exe", "CRS")

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 0 {
		t.Errorf("engine third-party tooling rowed at the depth boundary: %v", games)
	}
	assertKind(t, folder, GameDirEmpty)
}

// Unreal's CEF helper is never the game exe (the two skip-token lists must
// agree): an Engine/Binaries/Win64-only folder must not row.
func TestScan_UnrealCEFSubProcessNotCandidate(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "UEOnly")
	writePEExe(t, folder, "Engine/Binaries/Win64/UnrealCEFSubProcess.exe", "CEF")

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 0 {
		t.Errorf("UnrealCEFSubProcess rowed: %v", games)
	}
	assertKind(t, folder, GameDirEmpty)
}

// A Wii U dump with its emulator inside: the game rows at its root with
// the emulator exe as its launch binary and the folder as its title —
// the emulator itself (like Proton or Wine) is tooling, never the game.
func TestScan_EmulatorDirRowsAsGameAtRoot(t *testing.T) {
	root := t.TempDir()
	zelda := filepath.Join(root, "The Legend of Zelda - Breath of the Wild")
	writePEExe(t, zelda, "cemu/cemu.exe", "Cemu")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, zelda)
	if rows[canonicalPath(zelda)] != "The Legend of Zelda - Breath of the Wild" {
		t.Errorf("title = %q, want the folder (the emulator is tooling, not the game)", rows[canonicalPath(zelda)])
	}
	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(games[0].ExePath) != "cemu.exe" {
		t.Errorf("ExePath = %q, want the emulator exe for launching", games[0].ExePath)
	}
}

// Emulator names are not game titles, whether from PE metadata or stems.
func TestGameTitle_EmulatorNamesRejected(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"cemu", "yuzu", "ryujinx", "dolphin", "pcsx2", "rpcs3", "xenia", "citra", "retroarch"} {
		exe := writeNamedPE(t, dir, name+".exe", name)
		if got := GameTitle(exe, "Real Game Folder"); got != "Real Game Folder" {
			t.Errorf("GameTitle(%s) = %q, want folder (emulator is tooling)", name, got)
		}
	}
}
