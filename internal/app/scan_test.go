package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

func writeScanFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// mkSteamRoot builds a fixture Steam root whose libraryfolders.vdf points at
// itself; mkSteamGame then adds one appmanifest + install dir per game.
func mkSteamRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeScanFile(t, filepath.Join(root, "steamapps", "libraryfolders.vdf"),
		[]byte(`"libraryfolders" { "0" { "path" "`+root+`" } }`))
	return root
}

func mkSteamGame(t *testing.T, steamRoot, appID, name, dirName string) string {
	t.Helper()
	writeScanFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_"+appID+".acf"),
		[]byte(`"AppState" { "appid" "`+appID+`" "name" "`+name+`" "installdir" "`+dirName+`" }`))
	gameRoot := filepath.Join(steamRoot, "steamapps", "common", dirName)
	if err := os.MkdirAll(gameRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	return gameRoot
}

// mkManualGame creates a game directory the recursive scanner accepts:
// one subdirectory of root carrying a platform-appropriate binary.
func mkManualGame(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	bin := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	writeScanFile(t, bin, []byte("GAME"))
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		if err := os.Chmod(bin, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func entriesByName(entries []LibraryEntry) map[string]LibraryEntry {
	out := map[string]LibraryEntry{}
	for _, e := range entries {
		out[e.Game.Name] = e
	}
	return out
}

func TestScanLibraryMultiStore(t *testing.T) {
	steamRoot := mkSteamRoot(t)
	mkSteamGame(t, steamRoot, "100", "Steam Game", "SteamGame")
	extraRoot := t.TempDir()
	mkManualGame(t, extraRoot, "GameTwo")

	entries, err := ScanAllLibraries(context.Background(), nil, ScanAllOptions{
		SteamRoot: steamRoot,
		ExtraDirs: []string{extraRoot},
	})
	if err != nil {
		t.Fatalf("ScanAllLibraries: %v", err)
	}
	for _, e := range entries {
		t.Logf("entry: store=%s name=%q appid=%s exe=%q compat=%q optiscaler=%q components=%v",
			e.Game.Store, e.Game.Name, e.Game.AppID, e.Game.ExePath, e.Game.CompatPrefix,
			e.OptiScalerVersion, e.ComponentVersions)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (steam + manual)", len(entries))
	}
	byName := entriesByName(entries)

	sg, ok := byName["Steam Game"]
	if !ok {
		t.Fatal("steam game missing")
	}
	if sg.Game.Store != domain.StoreSteam {
		t.Errorf("steam game store = %v, want StoreSteam", sg.Game.Store)
	}
	if sg.Game.AppID != "100" {
		t.Errorf("steam game appid = %q, want 100", sg.Game.AppID)
	}

	mg, ok := byName["GameTwo"]
	if !ok {
		t.Fatal("manual game missing")
	}
	if mg.Game.Store != domain.StoreManual {
		t.Errorf("manual game store = %v, want StoreManual", mg.Game.Store)
	}
	if mg.Game.ExePath == "" {
		t.Error("manual game ExePath empty, want resolved binary")
	}
}

func TestLibraryEntryComponentVersions(t *testing.T) {
	steamRoot := mkSteamRoot(t)

	managed := mkSteamGame(t, steamRoot, "100", "Versioned Game", "VersionedGame")
	writeScanFile(t, filepath.Join(managed, "versionedgame.exe"), []byte("GAME"))
	writeScanFile(t, filepath.Join(managed, "OptiScaler.dll"), []byte("OPT"))
	writeScanFile(t, filepath.Join(managed, "nvngx_dlss.dll"), testutil.FixedVersionPE(3, 7, 20, 0))
	writeScanFile(t, filepath.Join(managed, "amd_fidelityfx_dx12.dll"), testutil.FixedVersionPE(1, 0, 1, 41314))

	unmanaged := mkSteamGame(t, steamRoot, "200", "Unmanaged Game", "UnmanagedGame")
	writeScanFile(t, filepath.Join(unmanaged, "unmanagedgame.exe"), []byte("GAME"))
	writeScanFile(t, filepath.Join(unmanaged, "nvngx_dlss.dll"), testutil.FixedVersionPE(3, 7, 20, 0))

	entries, err := ScanAllLibraries(context.Background(), nil, ScanAllOptions{SteamRoot: steamRoot})
	if err != nil {
		t.Fatalf("ScanAllLibraries: %v", err)
	}
	byName := entriesByName(entries)

	v, ok := byName["Versioned Game"]
	if !ok {
		t.Fatal("managed game missing")
	}
	t.Logf("managed components: %v (OptiScaler %q)", v.ComponentVersions, v.OptiScalerVersion)
	if got := v.ComponentVersions["dlss"]; got != "DLSS 3.7.20" {
		t.Errorf("dlss component = %q, want %q", got, "DLSS 3.7.20")
	}
	if got := v.ComponentVersions["fsr"]; got != "FSR 3.1.4" {
		t.Errorf("fsr component = %q, want %q", got, "FSR 3.1.4")
	}

	u, ok := byName["Unmanaged Game"]
	if !ok {
		t.Fatal("unmanaged game missing")
	}
	if len(u.ComponentVersions) != 0 || u.OptiScalerVersion != "" {
		t.Errorf("unmanaged game enriched (components=%v optiscaler=%q); "+
			"PE parsing must be guarded to managed installs", u.ComponentVersions, u.OptiScalerVersion)
	}
}

func TestLibraryEntryOptiScalerVersionChain(t *testing.T) {
	steamRoot := mkSteamRoot(t)

	manifestGame := mkSteamGame(t, steamRoot, "100", "Chain Manifest", "ChainManifest")
	writeScanFile(t, filepath.Join(manifestGame, "cm.exe"), []byte("GAME"))
	writeScanFile(t, filepath.Join(manifestGame, "OptiScaler.dll"), []byte("OPT"))
	writeScanFile(t, filepath.Join(manifestGame, "manifest.json"), []byte(`{"name":"OptiScaler","version":"0.9.4"}`))
	writeScanFile(t, filepath.Join(manifestGame, "OptiScaler.log"), []byte("OptiScaler v0.9.3\n"))

	logGame := mkSteamGame(t, steamRoot, "200", "Chain Log", "ChainLog")
	writeScanFile(t, filepath.Join(logGame, "cl.exe"), []byte("GAME"))
	writeScanFile(t, filepath.Join(logGame, "OptiScaler.dll"), []byte("OPT"))
	writeScanFile(t, filepath.Join(logGame, "OptiScaler.log"), []byte("preamble\nOptiScaler v0.9.4-pre2 (build abc)\n"))

	iniGame := mkSteamGame(t, steamRoot, "300", "Chain Ini", "ChainIni")
	writeScanFile(t, filepath.Join(iniGame, "ci.exe"), []byte("GAME"))
	writeScanFile(t, filepath.Join(iniGame, "OptiScaler.dll"), []byte("OPT"))
	writeScanFile(t, filepath.Join(iniGame, "OptiScaler.ini"), []byte("[Upscalers]\n"))

	// Committed store manifest (no OptiScaler.dll): install state alone must
	// trigger enrichment.
	committedGame := mkSteamGame(t, steamRoot, "400", "Chain Committed", "ChainCommitted")
	writeScanFile(t, filepath.Join(committedGame, "cc.exe"), []byte("GAME"))
	writeScanFile(t, filepath.Join(committedGame, "manifest.json"), []byte(`{"name":"OptiScaler","version":"0.8.1"}`))
	installDir, err := installDirOf(committedGame)
	if err != nil {
		t.Fatalf("installDirOf: %v", err)
	}
	st := store.New(t.TempDir())
	if err := st.Save(&domain.Manifest{
		ID:            "committed1",
		SchemaVersion: domain.SchemaVersion,
		Status:        domain.StatusCommitted,
		GameRoot:      committedGame,
		InstallDir:    installDir,
	}); err != nil {
		t.Fatal(err)
	}

	entries, err := ScanAllLibraries(context.Background(), st, ScanAllOptions{SteamRoot: steamRoot})
	if err != nil {
		t.Fatalf("ScanAllLibraries: %v", err)
	}
	byName := entriesByName(entries)

	cases := []struct {
		game string
		want string
	}{
		{"Chain Manifest", "0.9.4"},     // manifest beats log
		{"Chain Log", "0.9.4-pre2"},     // log banner when no manifest
		{"Chain Ini", ""},               // ini proves install, not version
		{"Chain Committed", "0.8.1"},    // committed status triggers enrichment
	}
	for _, tc := range cases {
		e, ok := byName[tc.game]
		if !ok {
			t.Fatalf("%s missing from scan", tc.game)
		}
		t.Logf("%s: OptiScalerVersion=%q", tc.game, e.OptiScalerVersion)
		if e.OptiScalerVersion != tc.want {
			t.Errorf("%s: OptiScalerVersion = %q, want %q", tc.game, e.OptiScalerVersion, tc.want)
		}
	}
	if got := byName["Chain Committed"].Status; got != domain.StatusCommitted {
		t.Errorf("Chain Committed status = %q, want committed", got)
	}
}
