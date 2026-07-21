package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/testutil"
)

// writeTitleExe writes a synthetic PE without any StringFileInfo (no usable
// metadata) at dir/name.exe, for the stem/folder fallback chain.
func writeTitleExe(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	pe := testutil.StringInfoPE(false, map[string]string{}, [4]uint16{1, 0, 0, 0})
	if err := os.WriteFile(p, pe, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeNamedPE writes a synthetic PE with the given ProductName.
func writeNamedPE(t *testing.T, dir, name, productName string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	pe := testutil.StringInfoPE(false, map[string]string{"ProductName": productName}, [4]uint16{1, 0, 0, 0})
	if err := os.WriteFile(p, pe, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The user's contract: binary metadata FIRST, then the exe name, then the
// directory name.
func TestGameTitle_TitlePriorityChain(t *testing.T) {
	dir := t.TempDir()

	// PE metadata beats everything.
	pe := writeNamedPE(t, dir, "whatever.exe", "Real PE Title")
	if got := GameTitle(pe, "Some Folder"); got != "Real PE Title" {
		t.Errorf("metadata: GameTitle = %q, want %q", got, "Real PE Title")
	}

	// No metadata → exe stem when it is more informative than the folder.
	stem := writeTitleExe(t, dir, "StarWarsJediFallenOrder.exe")
	if got := GameTitle(stem, "SwGame"); got != "Star Wars Jedi Fallen Order" {
		t.Errorf("stem: GameTitle = %q, want %q", got, "Star Wars Jedi Fallen Order")
	}

	// Platform suffixes are stripped before judging the stem.
	suffixed := writeTitleExe(t, dir, "CoolGame_x64_rwdi.exe")
	if got := GameTitle(suffixed, "stuff"); got != "Cool Game" {
		t.Errorf("strip: GameTitle = %q, want %q", got, "Cool Game")
	}

	// A stem that merely echoes the folder keeps the folder's (nicer) form.
	echo := writeTitleExe(t, dir, "dead space.exe")
	if got := GameTitle(echo, "Dead Space Remake"); got != "Dead Space Remake" {
		t.Errorf("echo: GameTitle = %q, want %q", got, "Dead Space Remake")
	}

	// Generic stems carry no information: folder wins.
	for _, name := range []string{"game.exe", "x64.exe", "hl.exe", "launcher.exe", "12345.exe"} {
		generic := writeTitleExe(t, dir, name)
		if got := GameTitle(generic, "PlainFolder"); got != "PlainFolder" {
			t.Errorf("generic %s: GameTitle = %q, want %q", name, got, "PlainFolder")
		}
	}

	// Unreal's bootstrap placeholder is not a title: fall through the chain.
	boot := writeNamedPE(t, dir, "TempestRising-Win64-Shipping.exe", "BootstrapPackagedGame")
	if got := GameTitle(boot, "Tempest Rising"); got != "Tempest Rising" {
		t.Errorf("bootstrap: GameTitle = %q, want %q", got, "Tempest Rising")
	}

	// No exe at all → folder.
	if got := GameTitle("", "JustFolder"); got != "JustFolder" {
		t.Errorf("no exe: GameTitle = %q, want %q", got, "JustFolder")
	}
}
