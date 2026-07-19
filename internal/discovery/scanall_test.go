package discovery

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

func TestScanAll_MergesStoresDedupes(t *testing.T) {
	steamRoot := t.TempDir()
	writeFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+steamRoot+`" } }`)
	writeFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Steam Game" "installdir" "SteamGame" }`)
	steamGameDir := filepath.Join(steamRoot, "steamapps", "common", "SteamGame")
	if err := os.MkdirAll(steamGameDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manualRoot := t.TempDir()
	mkGameBin(t, filepath.Join(manualRoot, "GameTwo", "gametwo"), 1<<20)

	// Cross-store duplicate: a manual-root alias of the Steam game's install
	// dir. The Steam entry must win and appear exactly once.
	if runtime.GOOS != "windows" {
		if err := os.Symlink(steamGameDir, filepath.Join(manualRoot, "SteamGameAlias")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	}

	games, err := ScanAll(context.Background(), ScanOptions{
		SteamRoots:     []string{steamRoot},
		RecursiveRoots: []string{manualRoot},
	})
	if err != nil {
		t.Fatalf("ScanAll: %v", err)
	}

	byName := map[string]domain.Game{}
	for _, g := range games {
		t.Logf("merged game: %+v", g)
		if _, dup := byName[g.Name]; dup {
			t.Fatalf("duplicate game in merged result: %s", g.Name)
		}
		byName[g.Name] = g
	}
	if len(games) != 2 {
		t.Fatalf("got %d games, want 2 (steam + manual, alias deduped)", len(games))
	}

	sg, ok := byName["Steam Game"]
	if !ok {
		t.Fatal("steam game missing from merged result")
	}
	if sg.Store != domain.StoreSteam {
		t.Errorf("steam game store = %v, want Steam", sg.Store)
	}
	if sg.InstallDir != canonicalPath(steamGameDir) {
		t.Errorf("steam game dir = %q, want canonical %q", sg.InstallDir, canonicalPath(steamGameDir))
	}

	mg, ok := byName["GameTwo"]
	if !ok {
		t.Fatal("manual game missing from merged result")
	}
	if mg.Store != domain.StoreManual || mg.ExePath == "" {
		t.Errorf("manual game = %+v, want StoreManual with resolved exe", mg)
	}
}
