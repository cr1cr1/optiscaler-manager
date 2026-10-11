// Package shadercache finds and measures the shader caches that matter
// for stutter on Linux/Proton (issue 078). It never deletes anything:
// discovery reports CacheRefs, and the callers (clear, warm) act on the
// claimed paths. Clean-room from SCSKiller's product shape — discovery,
// status, actions — not its Windows-only engine.
package shadercache

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Kind names the cache family a ref belongs to.
type Kind int

const (
	// KindDXVK is a DXVK state cache (*.dxvk-cache), per game.
	KindDXVK Kind = iota
	// KindVKD3D is a vkd3d-proton shader cache, per game.
	KindVKD3D
	// KindSteamFoz is Steam's shader precache for one appid,
	// read-only: Steam owns those bytes, we only measure them.
	KindSteamFoz
	// KindUnclaimed is a game-owned cache dir of unknown format
	// (e.g. cache/0000_0004): sized for information, never cleared.
	KindUnclaimed
	// KindMesa and KindNvidia are the shared driver caches: global
	// blobs no pane can attribute to one game.
	KindMesa
	KindNvidia
)

// Scope tells whether a ref belongs to one game or every game.
type Scope int

const (
	ScopePerGame Scope = iota
	ScopeGlobal
)

// Ref is one discovered cache: what it is, where it lives, how big it
// is, and (for files) exactly when it last changed. Dir refs carry the
// dir's own mtime — an approximation of freshness, documented so no UI
// presents it as exact.
type Ref struct {
	Kind      Kind
	Scope     Scope
	Path      string
	SizeBytes int64
	ModTime   time.Time
}

// Input selects what Discover measures. Empty fields disable that
// source: no PrefixDir skips the prefix, no HomeDir skips the driver
// caches, no SteamAppID skips the precache.
type Input struct {
	GameDir    string
	PrefixDir  string
	HomeDir    string
	SteamRoot  string // default: HomeDir/.local/share/Steam
	SteamAppID string
}

// Discover measures every cache source for one game and returns the
// refs in pane order: per-game claims, precache, unclaimed, globals.
// An unreadable corner is skipped with a warning — discovery never
// fails the caller.
func Discover(in Input) []Ref {
	var refs []Ref
	if in.GameDir != "" {
		refs = append(refs, claimGameDir(in.GameDir)...)
	}
	if in.PrefixDir != "" {
		refs = append(refs, claimPrefix(in.PrefixDir)...)
	}
	if in.SteamAppID != "" {
		root := in.SteamRoot
		if root == "" && in.HomeDir != "" {
			root = filepath.Join(in.HomeDir, ".local", "share", "Steam")
		}
		if root != "" {
			if r, ok := sizeDir(KindSteamFoz, ScopePerGame,
				filepath.Join(root, "steamapps", "shadercache", in.SteamAppID)); ok {
				refs = append(refs, r)
			}
		}
	}
	if in.HomeDir != "" {
		for _, g := range []struct {
			kind Kind
			path string
		}{
			{KindMesa, filepath.Join(in.HomeDir, ".cache", "mesa_shader_cache")},
			{KindNvidia, filepath.Join(in.HomeDir, ".cache", "nvidia", "GLCache")},
		} {
			if r, ok := sizeDir(g.kind, ScopeGlobal, g.path); ok {
				refs = append(refs, r)
			}
		}
		// The Mesa sqlite sidecar is part of the same cache.
		if r, ok := sizeDir(KindMesa, ScopeGlobal,
			filepath.Join(in.HomeDir, ".cache", "mesa_shader_cache_db")); ok {
			refs = append(refs, r)
		}
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		return refs[i].Path < refs[j].Path
	})
	return refs
}

// noiseDirs are never shader caches: VCS metadata and the manager's own
// in-game backup stores (their stale copies are not active caches).
func noiseDir(name string) bool {
	return name == ".git" || name == "dlss-backups" || name == "optiscaler-backups"
}

// unclaimedDir reports whether name looks like an engine-owned cache
// dir of unknown format: only exact well-known names qualify, so a
// "saves" or "levels" dir never lands in the pane as a cache.
func unclaimedDir(name string) bool {
	switch strings.ToLower(name) {
	case "cache", "shadercache", "shaders":
		return true
	}
	return false
}

// claimGameDir claims the known cache files under gameDir and sizes the
// unknown-format cache dirs. A dir holding claimed files reports the
// files, never the dir — no byte is ever counted twice. One pass: the
// walk is full (noise dirs aside), so claimed files nested anywhere are
// found, and candidate sizes accumulate inline instead of re-walking
// million-file engine dirs.
func claimGameDir(gameDir string) []Ref {
	var refs []Ref
	var candidates []string
	sizes := map[string]int64{}
	under := func(path, dir string) bool {
		return strings.HasPrefix(path, dir+string(os.PathSeparator))
	}
	_ = filepath.WalkDir(gameDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("shadercache: skipping unreadable path")
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if path != gameDir && noiseDir(d.Name()) {
				return filepath.SkipDir
			}
			// Nested candidates fold into the outermost: one row per
			// tree, never double-counted.
			if path != gameDir && unclaimedDir(d.Name()) {
				nested := false
				for _, c := range candidates {
					if under(path, c) {
						nested = true
						break
					}
				}
				if !nested {
					candidates = append(candidates, path)
				}
			}
			return nil
		}
		for _, c := range candidates {
			if under(path, c) {
				if info, err := d.Info(); err == nil {
					sizes[c] += info.Size()
				}
			}
		}
		switch {
		case hasSuffixFold(d.Name(), ".dxvk-cache"):
			refs = append(refs, fileRef(KindDXVK, ScopePerGame, path, d))
		case strings.EqualFold(d.Name(), "vkd3d-proton.cache"):
			refs = append(refs, fileRef(KindVKD3D, ScopePerGame, path, d))
		}
		return nil
	})
	claimedUnder := func(dir string) bool {
		for _, r := range refs {
			if under(r.Path, dir) {
				return true
			}
		}
		return false
	}
	for _, dir := range candidates {
		if claimedUnder(dir) {
			continue
		}
		info, err := os.Stat(dir)
		if err != nil {
			continue
		}
		refs = append(refs, Ref{Kind: KindUnclaimed, Scope: ScopePerGame,
			Path: dir, SizeBytes: sizes[dir], ModTime: info.ModTime()})
	}
	return refs
}

// claimPrefix claims only shallow prefix caches: prefixes are huge,
// and cache files that matter (DXVK state, vkd3d) sit at or near the
// prefix root. Anything deeper is prefix noise.
func claimPrefix(prefixDir string) []Ref {
	entries, err := os.ReadDir(prefixDir)
	if err != nil {
		log.Warn().Err(err).Str("path", prefixDir).Msg("shadercache: skipping unreadable prefix")
		return nil
	}
	var refs []Ref
	for _, e := range entries {
		if e.IsDir() || noiseDir(e.Name()) {
			continue
		}
		p := filepath.Join(prefixDir, e.Name())
		switch {
		case hasSuffixFold(e.Name(), ".dxvk-cache"):
			refs = append(refs, fileRef(KindDXVK, ScopePerGame, p, e))
		case strings.EqualFold(e.Name(), "vkd3d-proton.cache"):
			refs = append(refs, fileRef(KindVKD3D, ScopePerGame, p, e))
		}
	}
	return refs
}

// fileRef builds a ref for one claimed file; a stat failure degrades
// to zero size rather than dropping the row (the file was just seen).
func fileRef(kind Kind, scope Scope, path string, d fs.DirEntry) Ref {
	info, err := d.Info()
	if err != nil {
		log.Warn().Err(err).Str("path", path).Msg("shadercache: cannot stat claimed file")
		return Ref{Kind: kind, Scope: scope, Path: path}
	}
	return Ref{Kind: kind, Scope: scope, Path: path,
		SizeBytes: info.Size(), ModTime: info.ModTime()}
}

// sizeDir sums one dir tree (false when absent — silently, since most
// sources are absent on most machines): the shared measurer for
// precache and global caches.
func sizeDir(kind Kind, scope Scope, dir string) (Ref, bool) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return Ref{}, false
	}
	var total int64
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("shadercache: skipping unreadable path")
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	if err != nil {
		return Ref{}, false
	}
	return Ref{Kind: kind, Scope: scope, Path: dir,
		SizeBytes: total, ModTime: info.ModTime()}, true
}

func hasSuffixFold(name, suffix string) bool {
	return len(name) >= len(suffix) &&
		strings.EqualFold(name[len(name)-len(suffix):], suffix)
}
