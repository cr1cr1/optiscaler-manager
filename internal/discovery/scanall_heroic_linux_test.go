//go:build linux

package discovery

import (
	"context"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

func TestScanAll_MergesHeroicRows(t *testing.T) {
	xdg, _ := heroicEnv(t)
	games := t.TempDir()
	hades := mkGameDir(t, games, "hades", "Hades.exe")
	writeHeroicJSON(t, nativeEpicJSON(xdg), map[string]any{
		"HadesApp": map[string]any{
			"app_name":     "HadesApp",
			"title":        "Hades (Epic)",
			"install_path": hades,
			"executable":   "Hades.exe",
			"platform":     "Windows",
		},
	})

	// A recursive root over the same install dir: the heroic row must win
	// (store probe runs before manual roots) and appear exactly once.
	manualRoot := games

	merged, err := ScanAll(context.Background(), ScanOptions{
		SteamRoots:     []string{},
		RecursiveRoots: []string{manualRoot},
	})
	if err != nil {
		t.Fatalf("ScanAll: %v", err)
	}

	var heroic, manual int
	for _, g := range merged {
		t.Logf("merged game: %+v", g)
		if g.InstallDir != canonicalPath(hades) {
			continue
		}
		switch g.Store {
		case domain.StoreEpic:
			heroic++
			if g.Name != "Hades (Epic)" || g.AppName != "HadesApp" {
				t.Errorf("heroic row = %+v, want heroic title + appname", g)
			}
		case domain.StoreManual:
			manual++
		}
	}
	if heroic != 1 {
		t.Errorf("heroic rows for hades dir = %d, want 1", heroic)
	}
	if manual != 0 {
		t.Errorf("manual duplicate for hades dir = %d, want 0 (heroic wins)", manual)
	}
}
