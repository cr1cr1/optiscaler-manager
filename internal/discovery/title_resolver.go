package discovery

import (
	"path/filepath"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gid"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
)

// TitleResult is one resolved display title with its provenance and the
// Steam app id when one was detected (offline resolution records the appid
// even when the title itself came from the PE/stem/folder tail — the
// enrich phase upgrades those rows to canonical store names).
type TitleResult struct {
	Name       string
	Source     domain.TitleSource
	SteamAppID string
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
				return TitleResult{Name: o, Source: domain.SourceOverride, SteamAppID: det.SteamAppID}
			}
		}
		if det.Title != "" {
			return TitleResult{Name: det.Title, Source: det.Source, SteamAppID: det.SteamAppID}
		}
		name, src := resolveGameTitle(exe, filepath.Base(dir))
		return TitleResult{Name: name, Source: src, SteamAppID: det.SteamAppID}
	}
}

// resolveGameTitle is GameTitle with source attribution.
func resolveGameTitle(exe, folder string) (string, domain.TitleSource) {
	if exe == "" {
		return folder, domain.SourceFolder
	}
	if title := pever.TitleFromFile(exe); title != "" {
		return title, domain.SourcePE
	}
	if stem := exeStemTitle(exe, folder); stem != folder {
		return stem, domain.SourceStem
	}
	return folder, domain.SourceFolder
}
