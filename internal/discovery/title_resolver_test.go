package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

func writeChainFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeChainPE(t *testing.T, path, productName string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	pe := testutil.StringInfoPE(false, map[string]string{"ProductName": productName}, [4]uint16{1, 0, 0, 0})
	if err := os.WriteFile(path, pe, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The chain at row creation: override > goggame/egstore/unity > PE > stem
// > folder, with a steam appid recorded whenever the file exists.
func TestChainResolver_Precedence(t *testing.T) {
	noOverride := func(string) string { return "" }

	t.Run("override beats everything", func(t *testing.T) {
		dir := t.TempDir()
		writeChainFile(t, filepath.Join(dir, "goggame-1.info"), `{"gameId":"1","name":"GOG Name","playTasks":[]}`)
		writeChainFile(t, filepath.Join(dir, "steam_appid.txt"), "111")
		exe := writeChainPE(t, filepath.Join(dir, "game.exe"), "PE Name")
		res := ChainResolver(func(string) string { return "Pinned Name" })
		got := res(dir, exe)
		if got != (TitleResult{Name: "Pinned Name", Source: domain.SourceOverride, SteamAppID: "111"}) {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("goggame beats PE", func(t *testing.T) {
		dir := t.TempDir()
		writeChainFile(t, filepath.Join(dir, "goggame-1.info"), `{"gameId":"1","name":"GOG Name","playTasks":[]}`)
		exe := writeChainPE(t, filepath.Join(dir, "game.exe"), "PE Name")
		got := ChainResolver(noOverride)(dir, exe)
		if got.Name != "GOG Name" || got.Source != domain.SourceGOGInfo {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("appid recorded with tail source", func(t *testing.T) {
		dir := t.TempDir()
		writeChainFile(t, filepath.Join(dir, "steam_appid.txt"), "2322010")
		exe := writeChainPE(t, filepath.Join(dir, "game.exe"), "God of War")
		got := ChainResolver(noOverride)(dir, exe)
		if got.Name != "God of War" || got.Source != domain.SourcePE || got.SteamAppID != "2322010" {
			t.Errorf("got %+v, want PE title + appid", got)
		}
	})

	t.Run("tail attribution", func(t *testing.T) {
		dir := t.TempDir()
		// No metadata → stem when informative.
		pe := testutil.StringInfoPE(false, map[string]string{}, [4]uint16{1, 0, 0, 0})
		writeChainFile(t, filepath.Join(dir, "StarWarsJediFallenOrder.exe"), string(pe))
		got := ChainResolver(noOverride)(dir, filepath.Join(dir, "StarWarsJediFallenOrder.exe"))
		if got.Name != "Star Wars Jedi Fallen Order" || got.Source != domain.SourceStem {
			t.Errorf("stem: got %+v", got)
		}
		// Generic stem → folder.
		got = ChainResolver(noOverride)(dir, filepath.Join(dir, "game.exe"))
		if got.Name != filepath.Base(dir) || got.Source != domain.SourceFolder {
			t.Errorf("folder: got %+v", got)
		}
		// No exe → folder.
		got = ChainResolver(noOverride)(dir, "")
		if got.Name != filepath.Base(dir) || got.Source != domain.SourceFolder {
			t.Errorf("no exe: got %+v", got)
		}
	})
}

// Rows created by the recursive scan carry the appid and the true title
// source, even when the title came from the PE/stem/folder tail.
func TestScanRecursive_RecordsAppIDAndSource(t *testing.T) {
	root := t.TempDir()
	writePEExe(t, root, "God of War Ragnarok/steam_appid.txt", "")
	writeChainFile(t, filepath.Join(root, "God of War Ragnarok", "steam_appid.txt"), "2322010")
	writePEExe(t, root, "God of War Ragnarok/GoWR.exe", "GoWR")

	games, err := ScanRecursive(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d rows, want 1", len(games))
	}
	g := games[0]
	if g.SteamAppID != "2322010" || g.TitleSource != domain.SourcePE || g.Name != "GoWR" {
		t.Errorf("row = %+v, want appid + PE source + PE title", g)
	}
}
