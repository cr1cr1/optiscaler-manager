package discovery

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// scanRows scans root and returns the rows keyed by canonical install dir,
// failing the test on any error.
func scanRows(t *testing.T, root string) map[string]string {
	t.Helper()
	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	for _, g := range games {
		rows[canonicalPath(g.InstallDir)] = g.Name
	}
	return rows
}

// assertOnlyRows fails unless rows holds exactly the wanted install dirs.
func assertOnlyRows(t *testing.T, rows map[string]string, wantDirs ...string) {
	t.Helper()
	if len(rows) != len(wantDirs) {
		t.Errorf("got %d rows %v, want exactly %d %v", len(rows), rows, len(wantDirs), wantDirs)
	}
	for _, d := range wantDirs {
		if _, ok := rows[canonicalPath(d)]; !ok {
			t.Errorf("missing row at %s; rows: %v", d, rows)
		}
	}
}

// Cyberpunk shape from the user's library: the game root holds its own
// binaries under bin/x64 and a redmod tool under tools/. Neither engine
// folder may row or make the root a container — the root is the game.
func TestScan_EngineFoldersDoNotStealTheGameRow(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Cyberpunk 2077")
	writePEExe(t, game, "bin/x64/Cyberpunk2077.exe", "Cyberpunk 2077")
	writePEExe(t, game, "tools/redmod/redmod.exe", "redmod tool")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, game)
	if rows[canonicalPath(game)] != "Cyberpunk 2077" {
		t.Errorf("title = %q, want the game exe's PE title", rows[canonicalPath(game)])
	}
}

// Days Gone shape: an Engine/ subtree holding third-party binaries is the
// game's own support tree — it must not force the parent into a container
// nor spawn rows (the user got a "CRS" row from Engine/Binaries/ThirdParty).
func TestScan_EngineSubtreeDoesNotForceContainer(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Days Gone Broken Road")
	writePEExe(t, game, "BendGame/Binaries/Win64/DaysGone.exe", "Days Gone")
	writePEExe(t, game, "Engine/Binaries/ThirdParty/CRS/crs.exe", "CRS")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, filepath.Join(game, "BendGame"))
}

// Witcher/Crysis/007 shapes: bin64-style folders and the GDK Retail folder
// hold the game's own exe — the game rows at its root, never the engine
// folder itself.
func TestScan_Bin64AndRetailRowAtGameRoot(t *testing.T) {
	root := t.TempDir()
	witcher := filepath.Join(root, "The Witcher 3 - Wild Hunt")
	writePEExe(t, witcher, "bin/x64_dx12/witcher3.exe", "The Witcher 3")
	crysis := filepath.Join(root, "Crysis Remastered")
	writePEExe(t, crysis, "Bin64/CrysisRemastered.exe", "Crysis")
	bond := filepath.Join(root, "007 First Light")
	writePEExe(t, bond, "Retail/007.exe", "007 First Light")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, witcher, crysis, bond)
}

// Steam's compatdata holds Proton prefixes full of Windows system exes
// (drive_c/windows ships explorer.exe & friends): none of that is a game.
func TestScan_CompatdataPrefixIsNotAGame(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "steamapps", "common", "RealGame")
	writePEExe(t, real, "RealGame.exe", "Real Game Title")
	pfx := filepath.Join(root, "steamapps", "compatdata", "12345", "pfx", "drive_c")
	writePEExe(t, pfx, "windows/explorer.exe", "Microsoft Windows")
	writePEExe(t, pfx, "windows/system32/dxdiag.exe", "DirectX Diagnostic Tool")
	writePEExe(t, pfx, "Program Files/SomeApp/app.exe", "Some App")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, real)
}

// Proton and the Steam Linux Runtimes sit next to real games in
// steamapps/common: platform tooling, never rows — and their payload dirs
// ("files") must not row either.
func TestScan_ProtonAndSteamRuntimeAreNotGames(t *testing.T) {
	root := t.TempDir()
	common := filepath.Join(root, "steamapps", "common")
	writePEExe(t, common, "RealGame/RealGame.exe", "Real Game Title")
	writePEExe(t, common, "Proton - Experimental/files/bin/wine.exe", "Wine")
	writePEExe(t, common, "Proton Hotfix/files/bin/wine.exe", "Wine")
	writePEExe(t, common, "Proton 9.0/files/bin/wine.exe", "Wine")
	writePEExe(t, common, "SteamLinuxRuntime_sniper/files/bin/pressure-vessel.exe", "PV")
	writeFile(t, filepath.Join(common, "Proton - Experimental", "proton"), "#!/usr/bin/env python3")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, filepath.Join(common, "RealGame"))
}

// steamapps plumbing (shadercache/downloading/temp/music/sourcemods) and
// redist/installer support dirs never contain rows.
func TestScan_SteamPlumbingAndRedistDirsAreNotGames(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "Library", "RealGame")
	writePEExe(t, real, "RealGame.exe", "Real Game Title")
	writePEExe(t, filepath.Join(root, "Library"), "shadercache/123/blob/blob.exe", "Blob")
	writePEExe(t, filepath.Join(root, "Library"), "downloading/456/partial.exe", "Partial")
	writePEExe(t, filepath.Join(root, "Library"), "_CommonRedist/DotNet/4.8/ndp48.exe", "Microsoft .NET")
	writePEExe(t, filepath.Join(root, "Library"), "_Redist/vcredist/vc.exe", "VC Runtime")
	writePEExe(t, filepath.Join(root, "Library"), "__Installer/directx/jun2010/dx.exe", "DirectX")
	writePEExe(t, filepath.Join(root, "Library"), "Steamworks Common Redistributables/vcredist/2022/vc.exe", "VC")

	rows := scanRows(t, filepath.Join(root, "Library"))
	assertOnlyRows(t, rows, real)
}

// The steamapps row from the user's cache was InstallDir=steamapps with a
// shadercache "exe": with candidacy + plumbing names fixed, a steamapps
// dir classifies as a container and only its games row.
func TestScan_SteamappsItselfNeverRows(t *testing.T) {
	root := t.TempDir()
	apps := filepath.Join(root, "steamapps")
	writePEExe(t, apps, "common/GameA/GameA.exe", "Game A Title")
	writePEExe(t, apps, "common/GameB/bin/GameB.exe", "Game B Title")
	writePEExe(t, apps, "compatdata/100/pfx/drive_c/windows/explorer.exe", "Explorer")
	writeFile(t, filepath.Join(apps, "appmanifest_100.acf"), "\"AppState\" {}")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, filepath.Join(apps, "common", "GameA"), filepath.Join(apps, "common", "GameB"))
	for d := range rows {
		if strings.HasSuffix(d, "steamapps") {
			t.Errorf("steamapps rowed: %v", rows)
		}
	}
}

// Homeworld's classic build keeps its exe in a folder literally named
// "exe"; Steam's "Steamworks Shared" depot holds shared redistributables.
// Both are binary containers, not games.
func TestScan_ExeFolderAndSteamworksSharedAreNotGames(t *testing.T) {
	root := t.TempDir()
	hw := filepath.Join(root, "Homeworld1Classic")
	writePEExe(t, hw, "exe/homeworld.exe", "Homeworld")
	writePEExe(t, filepath.Join(root, "Steamworks Shared"), "vcredist/2022/vc.exe", "VC Runtime")

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, hw)
}

// The exe ranking must see through decorative separators: "FarCry5.exe" is
// the main exe of "Far Cry 5" even though the raw strings differ by a
// space — a larger junk binary must not win.
func TestScan_NameMatchIgnoresSeparators(t *testing.T) {
	root := t.TempDir()
	fc := filepath.Join(root, "Far Cry 5")
	writePEExe(t, fc, "bin/FarCry5.exe", "Far Cry 5 PE Title")
	big := filepath.Join(fc, "bin", "zztool.exe")
	writeSized(t, big, 9<<20)

	rows := scanRows(t, root)
	assertOnlyRows(t, rows, fc)
	if rows[canonicalPath(fc)] != "Far Cry 5 PE Title" {
		t.Errorf("title = %q, want the name-matched exe's PE title (separator-insensitive ranking)", rows[canonicalPath(fc)])
	}
}
