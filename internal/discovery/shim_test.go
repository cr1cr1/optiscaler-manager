package discovery

import (
	"context"
	"path/filepath"
	"testing"
)

// UE bootstrap pattern: a tiny root-level exe that is only a launcher for
// the real shipping binary under <Project>/Binaries/Win64. The shim wins
// today because its stem matches the folder (nameScore 2) — the real
// binary must win so titles come from real metadata and launches hit the
// real exe.
func TestFindMainExe_PrefersShippingOverBootstrapShim(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Layers of Fear")
	writeSized(t, filepath.Join(game, "LayersOfFear.exe"), 1<<10)
	writeSized(t, filepath.Join(game, "Cron", "Binaries", "Win64", "LayersOfFear-Win64-Shipping.exe"), 9<<20)

	exe, err := findMainExe(context.Background(), game)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(exe) != "LayersOfFear-Win64-Shipping.exe" {
		t.Errorf("exe = %q, want the shipping binary over the bootstrap shim", exe)
	}
}

// Black Myth's shape (b1.exe shim + b1/Binaries/Win64 shipping) already
// worked via the size tiebreak — pin it.
func TestFindMainExe_BlackMythShape(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Black Myth Wukong")
	writeSized(t, filepath.Join(game, "b1.exe"), 1<<10)
	writeSized(t, filepath.Join(game, "b1", "Binaries", "Win64", "b1-Win64-Shipping.exe"), 9<<20)

	exe, err := findMainExe(context.Background(), game)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(exe) != "b1-Win64-Shipping.exe" {
		t.Errorf("exe = %q, want the shipping binary", exe)
	}
}

// A small real exe at the root must not be demoted for a bigger tool that
// merely shares its name prefix without the UE shipping signature.
func TestFindMainExe_SmallRealExeNotDemoted(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Foo")
	writeSized(t, filepath.Join(game, "foo.exe"), 1<<10)
	writeSized(t, filepath.Join(game, "bin", "fooeditor.exe"), 9<<20)

	exe, err := findMainExe(context.Background(), game)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(exe) != "foo.exe" {
		t.Errorf("exe = %q, want the real root exe (no UE shipping signature on the bigger tool)", exe)
	}
}

// A large root exe is the game itself — the shim rule is only for tiny
// launchers.
func TestFindMainExe_LargeRootExeStays(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "RealGame")
	writeSized(t, filepath.Join(game, "RealGame.exe"), 9<<20)
	writeSized(t, filepath.Join(game, "Binaries", "Win64", "RealGame-Win64-Shipping.exe"), 10<<20)

	exe, err := findMainExe(context.Background(), game)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(exe) != "RealGame.exe" {
		t.Errorf("exe = %q, want the large root exe (not a shim)", exe)
	}
}
