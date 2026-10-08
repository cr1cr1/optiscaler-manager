package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// ScanGame re-discovers and enriches ONE library game by install dir,
// through the same source-specific paths ScanAllLibraries fans out to —
// so a per-game rescan cannot drift from what a global scan would row.

func TestScanGameSteamGame(t *testing.T) {
	steamRoot := mkSteamRoot(t)
	gameRoot := mkSteamGame(t, steamRoot, "100", "Steam Game", "SteamGame")

	e, err := ScanGame(context.Background(), nil, gameRoot, ScanGameOptions{SteamRoot: steamRoot})
	if err != nil {
		t.Fatalf("ScanGame: %v", err)
	}
	if e.Game.Store != domain.StoreSteam {
		t.Errorf("store = %v, want StoreSteam", e.Game.Store)
	}
	if e.Game.AppID != "100" || e.Game.Name != "Steam Game" {
		t.Errorf("game = appid %q name %q, want 100 / Steam Game", e.Game.AppID, e.Game.Name)
	}
	wantDir, err := canonicalDir(gameRoot)
	if err != nil {
		t.Fatal(err)
	}
	if e.Game.InstallDir != wantDir {
		t.Errorf("InstallDir = %q, want %q", e.Game.InstallDir, wantDir)
	}
}

func TestScanGameExtraDirChild(t *testing.T) {
	extraRoot := t.TempDir()
	dir := mkManualGame(t, extraRoot, "GameTwo")

	e, err := ScanGame(context.Background(), nil, dir, ScanGameOptions{
		SteamRoot: t.TempDir(), // empty fixture root: no steam games
		ExtraDirs: []string{extraRoot},
	})
	if err != nil {
		t.Fatalf("ScanGame: %v", err)
	}
	if e.Game.Store != domain.StoreManual {
		t.Errorf("store = %v, want StoreManual", e.Game.Store)
	}
	// A scan-root child rows through the recursive scanner, exactly like
	// the global scan (manual_ id prefix, not the custom_ self-row one).
	if e.Game.AppID != "manual_GameTwo" {
		t.Errorf("appid = %q, want manual_GameTwo (recursive-scan identity)", e.Game.AppID)
	}
	if e.Game.Name != "GameTwo" {
		t.Errorf("name = %q, want GameTwo", e.Game.Name)
	}
}

func TestScanGameExtraDirSelfRow(t *testing.T) {
	dir := mkManualGame(t, t.TempDir(), "SoloGame")

	e, err := ScanGame(context.Background(), nil, dir, ScanGameOptions{
		SteamRoot: t.TempDir(),
		ExtraDirs: []string{dir}, // the dir is itself an added game dir
	})
	if err != nil {
		t.Fatalf("ScanGame: %v", err)
	}
	if e.Game.Store != domain.StoreManual {
		t.Errorf("store = %v, want StoreManual", e.Game.Store)
	}
	// A self-row rows through ManualEntryWithResolver, exactly like
	// mergeExtraDirs in the global scan (custom_ id prefix).
	if e.Game.AppID != "custom_SoloGame" {
		t.Errorf("appid = %q, want custom_SoloGame (mergeExtraDirs identity)", e.Game.AppID)
	}
}

func TestScanGameNotFound(t *testing.T) {
	ghost := filepath.Join(t.TempDir(), "ghost")
	_, err := ScanGame(context.Background(), nil, ghost, ScanGameOptions{
		SteamRoot: t.TempDir(),
		ExtraDirs: []string{t.TempDir()},
	})
	if !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("err = %v, want errors.Is(err, ErrGameNotFound)", err)
	}
}

// A game whose dir survives but lost every executable no longer classifies
// as a game: ScanGame reports it gone instead of rowing an exe-less shell.
func TestScanGameExelessDirNotFound(t *testing.T) {
	extraRoot := t.TempDir()
	dir := mkManualGame(t, extraRoot, "GameTwo")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := ScanGame(context.Background(), nil, dir, ScanGameOptions{
		SteamRoot: t.TempDir(),
		ExtraDirs: []string{extraRoot},
	})
	if !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("err = %v, want errors.Is(err, ErrGameNotFound)", err)
	}
}
