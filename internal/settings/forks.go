package settings

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Fork names one OptiScaler distribution source: a GitHub repository
// (owner/repo slug) plus the glob matched against its release asset names
// (e.g. "Optiscaler_*.7z" upstream, "OptiScaler-NR-*.zip" for the DLSSNR
// multipass fork). The archive format is inferred from the matched
// asset's suffix downstream.
type Fork struct {
	Slug         string `json:"slug"`
	AssetPattern string `json:"asset_pattern"`
}

// DefaultForkSlug is the upstream OptiScaler repository: the out-of-box
// distribution and the fallback every fork operation resets to.
const DefaultForkSlug = "optiscaler/OptiScaler"

// builtinForks seeds Settings.Forks: upstream first, then the first
// supported alternative. Users may add or delete their own entries; the
// upstream entry itself can never be removed (RemoveFork refuses it and
// Load restores it when a hand-edited file drops it).
var builtinForks = []Fork{
	{Slug: DefaultForkSlug, AssetPattern: "Optiscaler_*.7z"},
	{Slug: "jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass", AssetPattern: "OptiScaler-NR-*.zip"},
}

// slugShape is the GitHub owner/repo form: alphanumerics, dash, dot, and
// underscore per segment, exactly one slash.
var slugShape = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// Active resolves ActiveFork to its fork entry. An empty or unknown slug
// falls back to upstream, so a hand-edited settings file can never leave
// the session without a usable distribution source.
func (s Settings) Active() Fork {
	for _, f := range s.Forks {
		if f.Slug == s.ActiveFork {
			return f
		}
	}
	return Fork{Slug: DefaultForkSlug, AssetPattern: builtinForks[0].AssetPattern}
}

// AddFork appends a fork after validating it: a well formed owner/repo
// slug, a non-empty glob pattern containing a wildcard, and no duplicate
// slug.
func (s *Settings) AddFork(f Fork) error {
	if !slugShape.MatchString(f.Slug) {
		return fmt.Errorf("settings: fork slug %q must have the owner/repo form", f.Slug)
	}
	if f.AssetPattern == "" || !strings.Contains(f.AssetPattern, "*") {
		return fmt.Errorf("settings: fork %q asset pattern %q must be a glob containing *", f.Slug, f.AssetPattern)
	}
	for _, e := range s.Forks {
		if e.Slug == f.Slug {
			return fmt.Errorf("settings: fork %q already exists", f.Slug)
		}
	}
	s.Forks = append(s.Forks, f)
	return nil
}

// RemoveFork deletes a fork by slug. The upstream entry is refused (the
// app always needs a fallback source); removing the active fork resets
// the selection to upstream.
func (s *Settings) RemoveFork(slug string) error {
	if slug == DefaultForkSlug {
		return fmt.Errorf("settings: the upstream fork %q cannot be removed", slug)
	}
	for i, f := range s.Forks {
		if f.Slug == slug {
			s.Forks = append(s.Forks[:i], s.Forks[i+1:]...)
			if s.ActiveFork == slug {
				s.ActiveFork = DefaultForkSlug
			}
			return nil
		}
	}
	return fmt.Errorf("settings: fork %q not found", slug)
}

// ForkKey maps a fork slug to a single safe directory name for cache
// namespacing. The "/" separator becomes a doubled underscore so a slug
// can never collide with a single-underscore owner or repo name
// ("a/b" → "a__b" ≠ "a_b" → "a_b").
func ForkKey(slug string) string {
	return strings.ReplaceAll(slug, "/", "__")
}

// BundleCacheDir is the fork's cache namespace under the cache root:
// releases.json, cooldown.json, and the <tag>/ bundle dirs all live here,
// so same-named tags from different distributions can never collide. An
// empty slug means the upstream distribution.
func BundleCacheDir(cacheDir, slug string) string {
	if slug == "" {
		slug = DefaultForkSlug
	}
	return filepath.Join(cacheDir, "optiscaler", ForkKey(slug))
}

// normalizeForks guarantees the built-ins after Load: an empty list
// (legacy file) seeds both built-ins, a hand-edited list missing upstream
// gets it prepended; a dangling ActiveFork resets to upstream.
func (s *Settings) normalizeForks() {
	if len(s.Forks) == 0 {
		s.Forks = append([]Fork(nil), builtinForks...)
	} else {
		found := false
		for _, f := range s.Forks {
			if f.Slug == DefaultForkSlug {
				found = true
				break
			}
		}
		if !found {
			s.Forks = append([]Fork{builtinForks[0]}, s.Forks...)
		}
	}
	activeKnown := false
	for _, f := range s.Forks {
		if f.Slug == s.ActiveFork {
			activeKnown = true
			break
		}
	}
	if s.ActiveFork == "" || !activeKnown {
		s.ActiveFork = DefaultForkSlug
	}
}
