//go:build linux

package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompatPrefix_Linux(t *testing.T) {
	steamRoot := t.TempDir()
	writeFile(t, filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf"),
		`"libraryfolders" { "0" { "path" "`+steamRoot+`" } }`)

	writeFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_100.acf"),
		`"AppState" { "appid" "100" "name" "Warp Game" "installdir" "ProtonGame" }`)
	writeFile(t, filepath.Join(steamRoot, "steamapps", "appmanifest_200.acf"),
		`"AppState" { "appid" "200" "name" "Flat Game" "installdir" "NativeGame" }`)
	for _, dir := range []string{"ProtonGame", "NativeGame"} {
		if err := os.MkdirAll(filepath.Join(steamRoot, "steamapps", "common", dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Only app 100 has a Proton prefix (created after its first Proton run).
	pfx := filepath.Join(steamRoot, "steamapps", "compatdata", "100", "pfx")
	if err := os.MkdirAll(pfx, 0o755); err != nil {
		t.Fatal(err)
	}

	games, err := ScanSteam(steamRoot)
	if err != nil {
		t.Fatalf("ScanSteam: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("got %d games, want 2", len(games))
	}
	for _, g := range games {
		t.Logf("game %s CompatPrefix=%q", g.AppID, g.CompatPrefix)
		switch g.AppID {
		case "100":
			if g.CompatPrefix != pfx {
				t.Errorf("app 100 CompatPrefix = %q, want %q", g.CompatPrefix, pfx)
			}
		case "200":
			if g.CompatPrefix != "" {
				t.Errorf("app 200 CompatPrefix = %q, want empty", g.CompatPrefix)
			}
		}
	}
}
