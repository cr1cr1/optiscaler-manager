package discovery

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const modernLibraryFoldersVDF = `
"libraryfolders"
{
	"0"
	{
		"path"		"/home/user/.steam/steam"
		"label"		""
		"contentid"		"111"
		"totalsize"		"0"
		"apps"
		{
			"1091500"		"50000000000"
		}
	}
	"1"
	{
		"path"		"/mnt/games/SteamLibrary"
		"label"		""
		"apps"
		{
		}
	}
}
`

const legacyLibraryFoldersVDF = `
"LibraryFolders"
{
	"TimeNextStatsReport"		"1234567890"
	"ContentStatsID"		"-987654321"
	"1"		"D:\\SteamLibrary"
	"2"		"/mnt/old/SteamLibrary"
}
`

func TestParseLibraryFolders(t *testing.T) {
	t.Run("modern nested form", func(t *testing.T) {
		got, err := ParseLibraryFolders(strings.NewReader(modernLibraryFoldersVDF))
		if err != nil {
			t.Fatalf("ParseLibraryFolders: %v", err)
		}
		want := []string{"/home/user/.steam/steam", "/mnt/games/SteamLibrary"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		t.Logf("modern form: %v", got)
	})

	t.Run("legacy flat form", func(t *testing.T) {
		got, err := ParseLibraryFolders(strings.NewReader(legacyLibraryFoldersVDF))
		if err != nil {
			t.Fatalf("ParseLibraryFolders: %v", err)
		}
		want := []string{`D:\\SteamLibrary`, "/mnt/old/SteamLibrary"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		t.Logf("legacy form: %v", got)
	})

	t.Run("invalid VDF errors", func(t *testing.T) {
		_, err := ParseLibraryFolders(strings.NewReader("this is { not vdf"))
		if err == nil {
			t.Fatal("expected error for invalid VDF, got nil")
		}
		t.Logf("invalid input error: %v", err)
	})

	t.Run("missing libraryfolders key errors", func(t *testing.T) {
		_, err := ParseLibraryFolders(strings.NewReader(`"somethingelse" { "a" "b" }`))
		if err == nil {
			t.Fatal("expected error for missing libraryfolders key, got nil")
		}
		t.Logf("missing key error: %v", err)
	})
}

const appmanifestACF = `
"AppState"
{
	"appid"		"1091500"
	"Universe"		"1"
	"LauncherPath"		"/steam/steamapps/common/Cyberpunk 2077/bin/x64/Cyberpunk2077.exe"
	"name"		"Cyberpunk 2077"
	"StateFlags"		"4"
	"installdir"		"Cyberpunk 2077"
	"LastUpdated"		"1700000000"
	"SizeOnDisk"		"70000000000"
}
`

func TestParseAppmanifest(t *testing.T) {
	t.Run("parses AppState fields", func(t *testing.T) {
		appID, name, installDir, err := ParseAppmanifest(strings.NewReader(appmanifestACF))
		if err != nil {
			t.Fatalf("ParseAppmanifest: %v", err)
		}
		if appID != "1091500" || name != "Cyberpunk 2077" || installDir != "Cyberpunk 2077" {
			t.Fatalf("got (%q, %q, %q)", appID, name, installDir)
		}
		t.Logf("appid=%s name=%q installdir=%q", appID, name, installDir)
	})

	t.Run("missing installdir errors", func(t *testing.T) {
		acf := `"AppState" { "appid" "42" "name" "NoDir Game" }`
		_, _, _, err := ParseAppmanifest(strings.NewReader(acf))
		if err == nil {
			t.Fatal("expected error for missing installdir, got nil")
		}
		t.Logf("missing installdir error: %v", err)
	})

	t.Run("missing AppState errors", func(t *testing.T) {
		_, _, _, err := ParseAppmanifest(strings.NewReader(`"Other" { "appid" "1" }`))
		if err == nil {
			t.Fatal("expected error for missing AppState, got nil")
		}
		t.Logf("missing AppState error: %v", err)
	})
}

// writeFile writes content to path, creating parent dirs.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestScanSteam(t *testing.T) {
	steamRoot := t.TempDir()
	libB := t.TempDir()

	// libraryfolders.vdf lists the root library itself plus a second one;
	// ScanSteam must dedupe the root.
	vdf := `"libraryfolders"
{
	"0" { "path" "` + steamRoot + `" }
	"1" { "path" "` + libB + `" }
}
`
	writeFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"), vdf)

	// Root library: one good game.
	writeFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`)
	if err := os.MkdirAll(filepath.Join(steamRoot, "steamapps", "common", "GameOne"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Second library: one good game, one broken manifest, one missing installdir.
	writeFile(t, filepath.Join(libB, "steamapps", "appmanifest_200.acf"),
		`"AppState" { "appid" "200" "name" "Game Two" "installdir" "GameTwo" }`)
	if err := os.MkdirAll(filepath.Join(libB, "steamapps", "common", "GameTwo"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(libB, "steamapps", "appmanifest_broken.acf"), "not { valid vdf")
	writeFile(t, filepath.Join(libB, "steamapps", "appmanifest_300.acf"),
		`"AppState" { "appid" "300" "name" "Ghost" "installdir" "NotOnDisk" }`)

	games, err := ScanSteam(steamRoot)
	if err != nil {
		t.Fatalf("ScanSteam: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("got %d games %+v, want 2", len(games), games)
	}

	type want struct{ name, installDir, lib string }
	wants := map[string]want{
		"100": {"Game One", filepath.Join(steamRoot, "steamapps", "common", "GameOne"), steamRoot},
		"200": {"Game Two", filepath.Join(libB, "steamapps", "common", "GameTwo"), libB},
	}
	for _, g := range games {
		t.Logf("game: %+v", g)
		w, ok := wants[g.AppID]
		if !ok {
			t.Errorf("unexpected appid %q", g.AppID)
			continue
		}
		if g.Name != w.name || g.InstallDir != w.installDir || g.LibraryPath != w.lib {
			t.Errorf("appid %s: got (%q, %q, %q), want (%q, %q, %q)",
				g.AppID, g.Name, g.InstallDir, g.LibraryPath, w.name, w.installDir, w.lib)
		}
	}
}

func TestScanSteamNoLibraryReadable(t *testing.T) {
	// No steamapps dir at all: no library can be read.
	if _, err := ScanSteam(t.TempDir()); err == nil {
		t.Fatal("expected error when no library is readable, got nil")
	} else {
		t.Logf("no libraries error: %v", err)
	}
}

func TestScanSteamSkipsNonGames(t *testing.T) {
	steamRoot := t.TempDir()
	writeFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+steamRoot+`" } }`)

	manifests := map[string]string{
		"appmanifest_100.acf":    `"AppState" { "appid" "100" "name" "Game One" "installdir" "GameOne" }`,
		"appmanifest_228980.acf": `"AppState" { "appid" "228980" "name" "Steamworks Common Redistributables" "installdir" "SteamworksShared" }`,
		"appmanifest_1391110.acf": `"AppState" { "appid" "1391110" "name" "Steam Linux Runtime 2.0 (soldier)" "installdir" "SteamLinuxRuntime_soldier" }`,
		"appmanifest_1887720.acf": `"AppState" { "appid" "1887720" "name" "Proton Experimental" "installdir" "ProtonExp" }`,
		"appmanifest_250820.acf": `"AppState" { "appid" "250820" "name" "SteamVR" "installdir" "SteamVR" }`,
		"appmanifest_250900.acf": `"AppState" { "appid" "250900" "name" "Wallpaper Engine" "installdir" "Wallpaper" }`,
	}
	for name, content := range manifests {
		writeFile(t, filepath.Join(steamRoot, "steamapps", name), content)
	}
	for _, dir := range []string{"GameOne", "SteamworksShared", "SteamLinuxRuntime_soldier", "ProtonExp", "SteamVR", "Wallpaper"} {
		if err := os.MkdirAll(filepath.Join(steamRoot, "steamapps", "common", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	games, err := ScanSteam(steamRoot)
	if err != nil {
		t.Fatalf("ScanSteam: %v", err)
	}
	if len(games) != 1 || games[0].Name != "Game One" {
		names := []string{}
		for _, g := range games {
			names = append(names, g.Name)
		}
		t.Fatalf("got %v, want only Game One", names)
	}
	t.Log("non-game Steam infrastructure excluded")
}
