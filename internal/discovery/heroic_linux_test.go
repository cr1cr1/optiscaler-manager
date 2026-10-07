//go:build linux

package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// heroicEnv isolates the probe's config roots: XDG_CONFIG_HOME for the
// native install, HOME for the Flatpak one.
func heroicEnv(t *testing.T) (xdg, home string) {
	t.Helper()
	root := t.TempDir()
	xdg = filepath.Join(root, "xdg")
	home = filepath.Join(root, "home")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HOME", home)
	return xdg, home
}

func writeHeroicJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkGameDir(t *testing.T, root, name string, files ...string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		// 4+ bytes: isBinaryMagic ReadAt()s a full 4-byte magic.
		if err := os.WriteFile(p, []byte("MZ\x90\x00"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func nativeEpicJSON(xdg string) string {
	return filepath.Join(xdg, "heroic", "legendaryConfig", "legendary", "installed.json")
}

func flatpakGOGJSON(home string) string {
	return filepath.Join(home, ".var", "app", "com.heroicgameslauncher.hgl", "config", "heroic", "gog_store", "installed.json")
}

func TestHeroicGamesFindsEpicNative(t *testing.T) {
	xdg, _ := heroicEnv(t)
	games := t.TempDir()
	fort := mkGameDir(t, games, "fortnite", "FortniteGame/Binaries/Win64/Fortnite.exe")
	writeHeroicJSON(t, nativeEpicJSON(xdg), map[string]any{
		"Fortnite": map[string]any{
			"app_name":     "Fortnite",
			"title":        "Fortnite",
			"install_path": fort,
			"executable":   `FortniteGame\Binaries\Win64\Fortnite.exe`, // legendary stores windows separators
			"platform":     "Windows",
		},
	})

	found := heroicGames()
	if len(found) != 1 {
		t.Fatalf("games = %d, want 1: %+v", len(found), found)
	}
	g := found[0]
	if g.Store != domain.StoreEpic || g.AppName != "Fortnite" || g.AppID != "Fortnite" {
		t.Errorf("store/appname/appid = %v/%q/%q", g.Store, g.AppName, g.AppID)
	}
	wantExe := filepath.Join(fort, "FortniteGame", "Binaries", "Win64", "Fortnite.exe")
	if g.ExePath != wantExe {
		t.Errorf("ExePath = %q, want %q", g.ExePath, wantExe)
	}
	t.Logf("epic row: %+v", g)
}

func TestHeroicGamesFindsGOGFlatpakAndInfoFallback(t *testing.T) {
	_, home := heroicEnv(t)
	games := t.TempDir()
	// No executable recorded in installed.json: the probe must fall back
	// to the goggame-*.info play tasks Heroic's GOG installs ship.
	hades := mkGameDir(t, games, "hades", "x64/Hades.exe")
	gogInfo := `{"gameId":"1207658930","name":"Hades","playTasks":[{"isPrimary":true,"path":"x64/Hades.exe","category":"game"}]}`
	if err := os.WriteFile(filepath.Join(hades, "goggame-1207658930.info"), []byte(gogInfo), 0o644); err != nil {
		t.Fatal(err)
	}
	writeHeroicJSON(t, flatpakGOGJSON(home), map[string]any{
		"1207658930": map[string]any{
			"appName":      "1207658930",
			"title":        "Hades",
			"install_path": hades,
			"platform":     "windows",
		},
	})

	found := heroicGames()
	if len(found) != 1 {
		t.Fatalf("games = %d, want 1: %+v", len(found), found)
	}
	g := found[0]
	if g.Store != domain.StoreGOG || g.AppName != "" {
		t.Errorf("store = %v, AppName = %q (want gog, empty)", g.Store, g.AppName)
	}
	if g.ExePath != filepath.Join(hades, "x64", "Hades.exe") {
		t.Errorf("ExePath = %q, want goggame info fallback", g.ExePath)
	}
	t.Logf("gog row: %+v", g)
}

func TestHeroicGamesSkipsUnusableEntries(t *testing.T) {
	xdg, _ := heroicEnv(t)
	games := t.TempDir()
	mac := mkGameDir(t, games, "macgame", "game.exe")
	writeHeroicJSON(t, nativeEpicJSON(xdg), map[string]any{
		"MacGame": map[string]any{
			"app_name":     "MacGame",
			"title":        "Mac Build",
			"install_path": mac,
			"executable":   "game.exe",
			"platform":     "Mac",
		},
		"MissingDir": map[string]any{
			"app_name":     "MissingDir",
			"title":        "Deleted",
			"install_path": filepath.Join(games, "nope"),
			"executable":   "nope.exe",
		},
	})

	if found := heroicGames(); len(found) != 0 {
		t.Fatalf("games = %+v, want none (mac + missing dir skipped)", found)
	}
	t.Log("mac build and missing install dir both skipped")
}

func TestHeroicGamesRejectsExeEscape(t *testing.T) {
	xdg, _ := heroicEnv(t)
	games := t.TempDir()
	dir := mkGameDir(t, games, "esc")
	writeHeroicJSON(t, nativeEpicJSON(xdg), map[string]any{
		"Esc": map[string]any{
			"app_name":     "Esc",
			"title":        "Escapee",
			"install_path": dir,
			"executable":   `..\evil.exe`,
		},
	})

	found := heroicGames()
	if len(found) != 1 {
		t.Fatalf("games = %d, want 1 (game kept, exe rejected)", len(found))
	}
	if found[0].ExePath != "" {
		t.Errorf("ExePath = %q, want empty (escape rejected)", found[0].ExePath)
	}
	t.Logf("escape rejected, row kept: %+v", found[0])
}

func TestHeroicGamesBrokenFileDoesNotBlockOthers(t *testing.T) {
	xdg, home := heroicEnv(t)
	bad := nativeEpicJSON(xdg)
	if err := os.MkdirAll(filepath.Dir(bad), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte(`{"unterminated`), 0o644); err != nil {
		t.Fatal(err)
	}
	games := t.TempDir()
	hades := mkGameDir(t, games, "hades", "Hades.exe")
	writeHeroicJSON(t, flatpakGOGJSON(home), map[string]any{
		"1207658930": map[string]any{
			"appName":      "1207658930",
			"title":        "Hades",
			"install_path": hades,
			"executable":   "Hades.exe",
		},
	})

	found := heroicGames()
	if len(found) != 1 || found[0].Store != domain.StoreGOG {
		t.Fatalf("games = %+v, want the gog row despite the broken epic file", found)
	}
	t.Logf("broken epic file logged and skipped, gog row: %+v", found[0])
}
