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
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/classify"
	"github.com/cr1cr1/optiscaler-manager/internal/discovery"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/installer"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// ManualEntry builds a library entry for a user-supplied game directory
// (added via the directory picker, not launcher discovery).
func ManualEntry(dir string) (LibraryEntry, error) {
	root, err := canonicalDir(dir)
	if err != nil {
		return LibraryEntry{}, err
	}
	name := manualName(root)
	e := LibraryEntry{
		Game: domain.Game{
			AppID:      "custom_" + filepath.Base(root),
			Name:       name,
			InstallDir: root,
			Store:      domain.StoreManual,
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

// manualName prefers the main executable's PE StringFileInfo title over the
// folder name; unreadable or title-less executables keep the folder name.
func manualName(root string) string {
	folder := filepath.Base(root)
	exe, err := discovery.FindMainExe(context.Background(), root)
	if err != nil || exe == "" {
		return folder
	}
	data, err := pever.ReadBounded(exe, 128<<20)
	if err != nil {
		log.Debug().Err(err).Str("exe", exe).Msg("manual entry: exe unreadable for title")
		return folder
	}
	if title := pever.ExtractTitle(data); title != "" {
		return title
	}
	return folder
}

// ErrEACProtected is returned by Install when the game ships an anti-cheat
// launcher and the caller did not pass InstallOpts.EACOverride.
var ErrEACProtected = errors.New("game is EAC-protected")

// ErrStaleCache is returned by Install when the GitHub API is rate-limited
// and only stale cached release info is available, and the caller did not
// pass InstallOpts.AllowCached.
var ErrStaleCache = errors.New("github API rate-limited; refusing stale cached release info")

// ErrNoGames is returned by ScanAllLibraries when discovery finds zero
// games. Frontends treat it as a settled empty library, never a failure.
var ErrNoGames = errors.New("no games found")

// ErrNotManaged is returned by Uninstall/Rollback when the store holds no
// manifest for the game: it was never installed by this manager (or was
// already removed) and its files must be handled by hand. Frontends match
// with errors.Is.
var ErrNotManaged = errors.New("not installed by this manager")

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

	OptiScalerVersion string            // "" when not installed or unknown
	ComponentVersions map[string]string // "dlss"/"fsr"/"xess" → marketing name
}

// ScanAllOptions controls ScanAllLibraries. An empty SteamRoot means "probe
// the platform's Steam roots"; ExtraDirs lists manual roots whose
// subdirectories are individual games (settings.ExtraDirs). Progress, when
// non-nil, receives ticks in pipeline order: "discover" per probed root,
// then "enrich" per discovered game.
type ScanAllOptions struct {
	SteamRoot string
	ExtraDirs []string
	Progress  func(phase string, done, total int)
}

// ScanAllLibraries discovers games across every store the platform supports
// (Steam, Epic, GOG, manual roots) via discovery.ScanAll and enriches them
// with tech, EAC, install status, and versions. Games carry
// Store/AppName/ExePath/CompatPrefix straight from discovery. An empty
// result fails with ErrNoGames.
func ScanAllLibraries(ctx context.Context, st *store.Store, opts ScanAllOptions) ([]LibraryEntry, error) {
	var steamRoots []string
	if opts.SteamRoot != "" {
		steamRoots = []string{opts.SteamRoot}
	}
	games, err := discovery.ScanAll(ctx, discovery.ScanOptions{
		SteamRoots:     steamRoots,
		RecursiveRoots: opts.ExtraDirs,
		Progress: func(done, total int) {
			if opts.Progress != nil {
				opts.Progress("discover", done, total)
			}
		},
	})
	if err != nil {
		return nil, err
	}

	var manifests []*domain.Manifest
	if st != nil {
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
	for i, g := range games {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out = append(out, enrich(g, byInstallDir))
		if opts.Progress != nil {
			opts.Progress("enrich", i+1, len(games))
		}
	}
	if len(out) == 0 {
		return nil, ErrNoGames
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
	var m *domain.Manifest
	if dir, err := installDirOf(g.InstallDir); err == nil {
		e.InjectionDir = dir
		if mm, ok := byInstallDir[dir]; ok {
			e.Status = mm.Status
			e.ManifestID = mm.ID
			m = mm
		}
	}
	// A game with no store manifest may still carry an OptiScaler dropped in
	// by hand: probe the injection dir for a branded injection DLL. The
	// probe is bounded to unmanaged rows — manifests stay authoritative.
	if e.Status == "" && e.InjectionDir != "" {
		if found, version := pever.DetectOptiScaler(e.InjectionDir); found {
			e.Status = domain.StatusExternal
			e.OptiScalerVersion = version // "" when the evidence chain runs dry
		}
	}
	enrichVersions(&e, m)
	return e
}

// enrichVersions fills OptiScalerVersion/ComponentVersions for managed
// installs only (committed manifest or OptiScaler.dll present). External
// rows are skipped: their version already came from the bounded
// DetectOptiScaler probe above, and their component DLLs belong to
// OptiScaler's bundle, not the game. Parsing PEs for every plain game
// would multiply scan I/O for no benefit. When the on-disk evidence chain
// (manifest.json → log → ini) yields no version — e.g. a fresh install
// that has not run yet — the committed store manifest's resolved version
// is the fallback.
func enrichVersions(e *LibraryEntry, m *domain.Manifest) {
	if e.InjectionDir == "" || e.Status == domain.StatusExternal {
		return
	}
	managed := e.Status == domain.StatusCommitted ||
		fileExists(filepath.Join(e.InjectionDir, "OptiScaler.dll"))
	if !managed {
		return
	}
	e.OptiScalerVersion = pever.OptiScalerVersion(e.InjectionDir)
	if e.OptiScalerVersion == "" && m != nil {
		e.OptiScalerVersion = m.Resolved.Version
	}
	e.ComponentVersions = componentVersions(e.InjectionDir)
}

// componentVersions parses each detected upscaler DLL under dir and maps it
// to the vendor marketing name. Unparseable DLLs are skipped, never fatal.
func componentVersions(dir string) map[string]string {
	var out map[string]string
	for _, f := range classify.DirFiles(dir) {
		kind, ok := peverKind(f.Kind)
		if !ok {
			continue // e.g. DLSS-FG has no marketing table
		}
		key := strings.ToLower(f.Kind.String())
		if _, dup := out[key]; dup {
			continue
		}
		raw, err := pever.FileVersion(f.Path)
		if err != nil {
			log.Debug().Err(err).Str("dll", f.Path).Msg("upscaler DLL version unreadable, skipping")
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[key] = pever.MarketingName(kind, raw)
	}
	return out
}

func peverKind(k domain.Kind) (pever.Kind, bool) {
	switch k {
	case domain.KindDLSS:
		return pever.KindDLSS, true
	case domain.KindFSR:
		return pever.KindFSR, true
	case domain.KindXeSS:
		return pever.KindXeSS, true
	}
	return 0, false
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
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

// requireManaged maps a missing store manifest to ErrNotManaged (with
// install-dir context) so frontends can tell "not ours" apart from real
// store failures; other load errors pass through unchanged.
func requireManaged(st *store.Store, id, dir string) error {
	if _, err := st.Load(id); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotManaged, dir)
		}
		return err
	}
	return nil
}

// Uninstall reverses the committed install for a game root.
func Uninstall(ctx context.Context, st *store.Store, gameRoot string) (string, error) {
	id, dir, err := ManifestIDFor(gameRoot)
	if err != nil {
		return "", err
	}
	if err := requireManaged(st, id, dir); err != nil {
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
	if err := requireManaged(st, id, dir); err != nil {
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
