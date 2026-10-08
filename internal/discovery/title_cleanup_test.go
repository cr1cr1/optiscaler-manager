package discovery

import (
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// Launcher-shim PE titles must never surface as the game title: Remedy's
// Control.exe is a small launcher whose ProductName is "ControlLauncher"
// while the real game exes sit beside it. The chain falls through to the
// folder name — scene tags cleaned — and identify canonicalizes from
// there (issue 030).
func TestChainResolver_LauncherShimPEFallsThrough(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Control Ultimate Edition PROPER")
	writePEExe(t, dir, "Control.exe", "ControlLauncher")

	got := ChainResolver(nil)(dir, filepath.Join(dir, "Control.exe"))
	if got.Name != "Control Ultimate Edition" || got.Source != domain.SourceFolder {
		t.Errorf("ChainResolver = %+v, want the cleaned folder title, not the launcher shim's PE string", got)
	}
}

// The launcher guard is a suffix rule with a mandatory prefix: a game
// genuinely called "Launcher" keeps its PE title.
func TestChainResolver_LauncherSuffixNeedsPrefix(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Launcher")
	writePEExe(t, dir, "game.exe", "Launcher")

	got := ChainResolver(nil)(dir, filepath.Join(dir, "game.exe"))
	if got.Name != "Launcher" || got.Source != domain.SourcePE {
		t.Errorf("ChainResolver = %+v, want the PE title kept (nothing precedes the suffix)", got)
	}
}

// Scene/repack tags and version runs are stripped from display titles;
// subtitles and year parens are display data and stay (issue 030).
func TestCleanTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Control Ultimate Edition PROPER", "Control Ultimate Edition"},
		{"STAR WARS Jedi Fallen Order v1 0 10 0 MULTi13", "STAR WARS Jedi Fallen Order"},
		{"Some Game REPACK", "Some Game"},
		{"Game v2.5", "Game"},
		{"Game v2", "Game"},
		{"Layers of Fear (2016)", "Layers of Fear (2016)"},
		{"Riven - The sequel to Myst", "Riven - The sequel to Myst"},
		{"Spelunky HD", "Spelunky HD"},
		{"PROPER", "PROPER"}, // a title is never stripped to nothing
	}
	for _, tc := range cases {
		if got := cleanTitle(tc.in); got != tc.want {
			t.Errorf("cleanTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Folder titles ride the same cleanup when nothing richer resolved.
func TestChainResolver_FolderTitleCleaned(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Control Ultimate Edition PROPER")

	got := ChainResolver(nil)(dir, "")
	if got.Name != "Control Ultimate Edition" || got.Source != domain.SourceFolder {
		t.Errorf("ChainResolver = %+v, want the cleaned folder title", got)
	}
}
