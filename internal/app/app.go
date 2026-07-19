// Package app holds the orchestration shared by the CLI and the GUI:
// library scanning with install status, and the install/uninstall/rollback
// workflows. Domain packages do the work; this package sequences them.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/classify"
	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/installer"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// ManualEntry builds a library entry for a user-supplied game directory
// (added via the directory picker, not launcher discovery).
func ManualEntry(dir string) (LibraryEntry, error) {
	root, err := canonicalDir(dir)
	if err != nil {
		return LibraryEntry{}, err
	}
	name := filepath.Base(root)
	e := LibraryEntry{
		Game: domain.Game{
			AppID:      "custom_" + name,
			Name:       name,
			InstallDir: root,
		},
		EAC: installer.EACProtected(root),
	}
	tech := map[string]bool{}
	for _, c := range classify.Dir(root) {
		tech[c.Kind.String()] = true
	}
	for k := range tech {
		e.Tech = append(e.Tech, k)
	}
	sort.Strings(e.Tech)
	if st, err := os.Stat(root); err == nil {
		e.ModTime = st.ModTime()
	}
	if d, err := installDirOf(root); err == nil {
		e.InjectionDir = d
	}
	return e, nil
}

// ErrEACProtected is returned by Install when the game ships an anti-cheat
// launcher and the caller did not pass InstallOpts.EACOverride.
var ErrEACProtected = errors.New("game is EAC-protected")

// ErrStaleCache is returned by Install when the GitHub API is rate-limited
// and only stale cached release info is available, and the caller did not
// pass InstallOpts.AllowCached.
var ErrStaleCache = errors.New("github API rate-limited; refusing stale cached release info")

// LibraryEntry is one row of the game library: the discovered game enriched
// with upscaler tech, anti-cheat flag, install status, and directory mtime.
type LibraryEntry struct {
	Game         domain.Game
	Tech         []string
	EAC          bool
	Status       domain.Status // empty when never installed
	ManifestID   string
	InjectionDir string // resolved install dir (where injection + ini live)
	ModTime      time.Time
}

// ScanLibrary discovers games (one Steam root, or all auto-detected roots
// when steamRoot is empty) and enriches them. When st is non-nil, committed
// or interrupted installs are reflected in Status.
func ScanLibrary(ctx context.Context, st *store.Store, steamRoot string) ([]LibraryEntry, error) {
	roots := []string{}
	if steamRoot != "" {
		roots = append(roots, steamRoot)
	} else {
		roots = discovery.SteamRoots()
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no Steam installation found")
	}

	var manifests []*domain.Manifest
	if st != nil {
		var err error
		manifests, err = st.List()
		if err != nil {
			return nil, err
		}
	}
	byInstallDir := map[string]*domain.Manifest{}
	for _, m := range manifests {
		byInstallDir[m.InstallDir] = m
	}

	var out []LibraryEntry
	seen := map[string]bool{}
	for _, root := range roots {
		games, err := discovery.ScanSteam(root)
		if err != nil {
			continue // one bad root must not sink the scan; caller sees the rest
		}
		for _, g := range games {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if seen[g.InstallDir] {
				continue
			}
			seen[g.InstallDir] = true
			out = append(out, enrich(g, byInstallDir))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no games found")
	}
	return out, nil
}

func enrich(g domain.Game, byInstallDir map[string]*domain.Manifest) LibraryEntry {
	tech := map[string]bool{}
	for _, c := range classify.Dir(g.InstallDir) {
		tech[c.Kind.String()] = true
	}
	var kinds []string
	for k := range tech {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	e := LibraryEntry{Game: g, Tech: kinds, EAC: installer.EACProtected(g.InstallDir)}
	if st, err := os.Stat(g.InstallDir); err == nil {
		e.ModTime = st.ModTime()
	}
	if dir, err := installDirOf(g.InstallDir); err == nil {
		e.InjectionDir = dir
		if m, ok := byInstallDir[dir]; ok {
			e.Status = m.Status
			e.ManifestID = m.ID
		}
	}
	return e
}

// installDirOf mirrors the install-dir resolution for manifest lookup;
// failures mean "no manifest" rather than a scan error.
func installDirOf(gameRoot string) (string, error) {
	dir, err := discovery.ResolveInstallDir(gameRoot)
	if err != nil {
		return "", err
	}
	return filepath.Clean(dir), nil
}

// InstallOpts tunes the install workflow for the calling frontend.
type InstallOpts struct {
	AllowCached bool   // accept stale cached release info under rate limiting
	EACOverride bool   // install despite anti-cheat detection
	Requested   string // release tag to install; "latest" when empty
}

// Install runs resolve → download → transactional install for a game root.
// Cancellation is honored at every phase boundary (docs/safety.md).
func Install(ctx context.Context, st *store.Store, client *gh.Client, cacheDir, gameRoot string, opts InstallOpts) (*domain.Manifest, error) {
	// Cancel boundary (pre-resolve): no network, no writes.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := canonicalDir(gameRoot)
	if err != nil {
		return nil, err
	}
	installDir, err := discovery.ResolveInstallDir(root)
	if err != nil {
		return nil, err
	}
	if installer.EACProtected(root) && !opts.EACOverride {
		return nil, fmt.Errorf("%w: %s (ban risk)", ErrEACProtected, root)
	}

	requested := opts.Requested
	if requested == "" {
		requested = "latest"
	}
	resolved, fromCache, err := client.Resolve(ctx, requested)
	if err != nil {
		return nil, err
	}
	if fromCache && !opts.AllowCached {
		return nil, ErrStaleCache
	}

	bundleDir := filepath.Join(cacheDir, "optiscaler", resolved.Version)
	bundlePath := filepath.Join(bundleDir, resolved.AssetName)
	digest, err := fileSHA256(bundlePath)
	if err != nil {
		// Cancel boundary (pre-download): the cache missed; refuse to fetch
		// under a dead context.
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		bundlePath, digest, err = client.Download(ctx, resolved, bundleDir)
		if err != nil {
			return nil, err
		}
	}
	resolved.SHA256 = digest

	return installer.Install(ctx, st, installer.Request{
		GameRoot:         root,
		InstallDir:       installDir,
		ArchivePath:      bundlePath,
		RequestedVersion: requested,
		Resolved:         resolved,
	})
}

// ManifestIDFor maps a game root to (manifest ID, install dir).
func ManifestIDFor(gameRoot string) (id, installDir string, err error) {
	root, err := canonicalDir(gameRoot)
	if err != nil {
		return "", "", err
	}
	dir, err := discovery.ResolveInstallDir(root)
	if err != nil {
		return "", "", err
	}
	id, err = installer.ManifestIDFor(dir)
	if err != nil {
		return "", "", err
	}
	return id, dir, nil
}

// Uninstall reverses the committed install for a game root.
func Uninstall(ctx context.Context, st *store.Store, gameRoot string) (string, error) {
	id, dir, err := ManifestIDFor(gameRoot)
	if err != nil {
		return "", err
	}
	if err := installer.Uninstall(ctx, st, id); err != nil {
		return "", err
	}
	return dir, nil
}

// Rollback restores a game root after an interrupted or failed install.
func Rollback(ctx context.Context, st *store.Store, gameRoot string) (string, error) {
	id, dir, err := ManifestIDFor(gameRoot)
	if err != nil {
		return "", err
	}
	if err := installer.Rollback(ctx, st, id); err != nil {
		return "", err
	}
	return dir, nil
}

func canonicalDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", abs, err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return abs, nil
}

// fileSHA256 streams a file through SHA-256 and returns the hex digest.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
