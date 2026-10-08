package discovery

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gid"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
)

// TitleResult is one resolved display title with its provenance and the
// Steam app id when one was detected (offline resolution records the appid
// even when the title itself came from the PE/stem/folder tail — the
// enrich phase upgrades those rows to canonical store names).
type TitleResult struct {
	Name        string
	Source      domain.TitleSource
	SteamAppID  string
	EpicAppName string
}

// TitleResolver resolves the display title of one game directory; exe is
// the picked main executable ("" when none).
type TitleResolver func(dir, exe string) TitleResult

// ChainResolver builds the v0.8 identification chain: a user override
// beats everything, in-dir metadata (goggame/.egstore/Unity) beats the
// binary chain, and PE metadata → exe stem → folder name is the tail.
// override maps a directory to its pinned title ("" when none) and may be
// nil. A detected steam_appid.txt is always reported.
func ChainResolver(override func(dir string) string) TitleResolver {
	return func(dir, exe string) TitleResult {
		det := gid.Detect(dir, exe)
		if override != nil {
			if o := override(dir); o != "" {
				return TitleResult{Name: o, Source: domain.SourceOverride, SteamAppID: det.SteamAppID, EpicAppName: det.EpicAppName}
			}
		}
		if det.Title != "" {
			return TitleResult{Name: cleanTitle(det.Title), Source: det.Source, SteamAppID: det.SteamAppID, EpicAppName: det.EpicAppName}
		}
		name, src := resolveGameTitle(exe, filepath.Base(dir))
		return TitleResult{Name: cleanTitle(name), Source: src, SteamAppID: det.SteamAppID, EpicAppName: det.EpicAppName}
	}
}

var (
	// trailingVersionRe matches a version run at the end of a title:
	// "v1 0 10 0", "v2.5" (scene folder names carry them).
	trailingVersionRe = regexp.MustCompile(`(?i)\s+v\d+(?:[ ._]\d+)+$`)
	// sceneTagRe matches one release-scene tag token.
	sceneTagRe = regexp.MustCompile(`(?i)^(v\d+|multi\d+|proper|repack|internal|dirfix|readnfo|nuked)$`)
)

// cleanTitle strips release-scene noise from a display title: repack tags
// ("PROPER", "REPACK", "MULTi13") and trailing version runs that folder
// names and sloppy PE strings carry. Subtitles and year parens are
// display data and stay. A title is never stripped to nothing
// (issue 030).
func cleanTitle(name string) string {
	orig := strings.Join(strings.Fields(name), " ")
	name = orig
	for {
		before := name
		name = trailingVersionRe.ReplaceAllString(name, "")
		if toks := strings.Fields(name); len(toks) > 1 && sceneTagRe.MatchString(toks[len(toks)-1]) {
			name = strings.TrimSpace(name[:len(name)-len(toks[len(toks)-1])])
		}
		if name == before {
			break
		}
	}
	if name == "" {
		return orig
	}
	return name
}

// launcherShimTitle reports whether a PE title is a launcher shim's
// identity rather than the game's: Remedy's Control.exe bootstrapper
// reports "ControlLauncher" while the real game exes sit beside it, and
// RSI's "RSI Launcher" is tooling, not a game. Such titles fall through
// to the next resolver source (issue 030). The guard is suffix-only with
// a mandatory prefix, so a game genuinely called "Launcher" keeps its
// title (and a title override always remains the escape hatch).
func launcherShimTitle(title string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	return strings.HasSuffix(t, "launcher") && len(strings.TrimSuffix(t, "launcher")) >= 3
}

// resolveGameTitle is GameTitle with source attribution.
func resolveGameTitle(exe, folder string) (string, domain.TitleSource) {
	if exe == "" {
		return folder, domain.SourceFolder
	}
	if title := pever.TitleFromFile(exe); title != "" && !launcherShimTitle(title) {
		return title, domain.SourcePE
	}
	if stem := exeStemTitle(exe, folder); stem != folder {
		return stem, domain.SourceStem
	}
	return folder, domain.SourceFolder
}
