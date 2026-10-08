package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// ErrGameNotFound is returned by ScanGame when no discovery source resolves
// a game at the requested directory anymore (deleted, uninstalled from its
// store, or stripped of every executable). Frontends match with errors.Is;
// pruning the stale row stays the global scan's job.
var ErrGameNotFound = errors.New("game no longer found")

// ScanGameOptions mirrors ScanAllOptions minus the streaming callbacks: the
// per-game rescan re-discovers one game with the same sources and settings
// snapshot as a global scan.
type ScanGameOptions struct {
	SteamRoot string
	ExtraDirs []string
	Resolver  discovery.TitleResolver
}

// ScanGame re-discovers and enriches the single game at dir through the
// same source-specific paths ScanAllLibraries fans out to, in the same
// precedence: store sources first (Steam appmanifests and the launcher
// configs, matched by canonical install dir), then the manual paths — a
// scan-root child re-rows through the recursive scanner, a dir that is
// itself an added game dir re-rows through ManualEntryWithResolver
// (mirroring mergeExtraDirs, so its custom_ id stays stable). A game no
// source resolves yields ErrGameNotFound.
func ScanGame(ctx context.Context, st *store.Store, dir string, opts ScanGameOptions) (LibraryEntry, error) {
	root, err := canonicalDir(dir)
	if err != nil {
		return LibraryEntry{}, fmt.Errorf("%w: %s", ErrGameNotFound, dir)
	}
	var manifests []*domain.Manifest
	if st != nil {
		manifests, err = st.List()
		if err != nil {
			return LibraryEntry{}, err
		}
	}
	byInstallDir := map[string]*domain.Manifest{}
	for _, m := range manifests {
		byInstallDir[m.InstallDir] = m
	}

	var steamRoots []string
	if opts.SteamRoot != "" {
		steamRoots = []string{opts.SteamRoot}
	}
	var found *domain.Game
	// No recursive roots: the store fan-out is bounded manifest/config
	// reads, so matching by dir costs nothing next to a full discovery.
	_, err = discovery.ScanAll(ctx, discovery.ScanOptions{
		SteamRoots: steamRoots,
		Resolver:   opts.Resolver,
		OnGame: func(g domain.Game) {
			if found == nil && g.InstallDir == root {
				gg := g
				found = &gg
			}
		},
	})
	if err != nil {
		return LibraryEntry{}, err
	}
	if found != nil {
		return enrich(*found, byInstallDir), nil
	}

	res := opts.Resolver
	if res == nil {
		res = discovery.ChainResolver(nil)
	}
	for _, d := range opts.ExtraDirs {
		base, err := canonicalDir(d)
		if err != nil {
			continue
		}
		if base == root {
			// The dir is itself an added game dir: the same self-row path
			// mergeExtraDirs takes in a global scan.
			return ManualEntryWithResolver(root, st, res)
		}
		if pathWithin(base, root) {
			// A scan-root child: the same recursive-scan path, scoped to
			// the game's own dir (it classifies as the root game).
			games, err := discovery.ScanRecursiveWithResolver(ctx, root, res)
			if err != nil {
				return LibraryEntry{}, err
			}
			for _, g := range games {
				if g.InstallDir == root {
					return enrich(g, byInstallDir), nil
				}
			}
			return LibraryEntry{}, fmt.Errorf("%w: %s", ErrGameNotFound, dir)
		}
	}
	return LibraryEntry{}, fmt.Errorf("%w: %s", ErrGameNotFound, dir)
}

// pathWithin reports whether p sits strictly below base.
func pathWithin(base, p string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
