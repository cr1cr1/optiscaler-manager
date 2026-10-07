package discovery

import (
	"context"
	"path/filepath"
	"testing"
)

// Issue 028: UE packaged-build staging directories
// (IntermediateBuildDRM/WindowsNoEditor/<Game>/Binaries/Win64) are
// transparent engine folders, not games: the row belongs to the real game
// root even though the exe sits 5 directory levels below it (one past the
// old maxExeDepth record — Prey's 4).
func TestScanRecursive_UEStagingLayout_RowsRealRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "Bramble The Mountain King")
	writeFile(t, filepath.Join(root, "IntermediateBuildDRM", "WindowsNoEditor",
		"Bramble_TMK", "Binaries", "Win64", "Bramble_TMK-Win64-Shipping.exe"), "fake-pe")

	games, err := ScanRecursive(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("games = %d, want 1: %+v", len(games), games)
	}
	if want := canonicalPath(root); games[0].InstallDir != want {
		t.Errorf("InstallDir = %q, want the real game root %q (not the staging dir)", games[0].InstallDir, want)
	}
}

// Issue 028: a launcher-only install dir is not a game. The real launcher
// binary is skip-token'd ("launcher"), so without the elevate skip the
// updater helper (resources/elevate.exe) wins and the tooling dir rows.
func TestScanRecursive_ElevateHelperOnly_NoRow(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "RSI Launcher")
	writeFile(t, filepath.Join(dir, "RSI Launcher.exe"), "fake-pe")
	writeFile(t, filepath.Join(dir, "resources", "elevate.exe"), "fake-pe")
	writeFile(t, filepath.Join(dir, "resources", "VC_redist.x64.exe"), "fake-pe")

	games, err := ScanRecursive(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 0 {
		t.Fatalf("games = %+v, want none (launcher tooling, no game installed)", games)
	}
}

// Issue 028: UE packaged-build staging names are engine folders
// (transparent: never rows, never containers, still walkable).
func TestEngineFolderName_UEStagingNames(t *testing.T) {
	for _, name := range []string{
		"WindowsNoEditor", "WindowsClient", "WindowsServer",
		"IntermediateBuild", "IntermediateBuildDRM",
	} {
		if !engineFolderName(name) {
			t.Errorf("engineFolderName(%q) = false, want true (UE staging dir)", name)
		}
	}
}
