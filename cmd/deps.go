package optiscalermanager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/app"
	"github.com/cr1cr1/optiscaler-manager/internal/covers"
	"github.com/cr1cr1/optiscaler-manager/internal/dlss"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/launch"
	"github.com/cr1cr1/optiscaler-manager/internal/settings"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// Deps carries everything a subcommand needs, injected by the root command
// (and by tests, which substitute buffers, temp stores, and httptest-backed
// GitHub clients).
type Deps struct {
	Out      io.Writer
	ErrOut   io.Writer
	Store    *store.Store
	DataRoot string
	CacheDir string
	GH       *gh.Client
	// NewGH builds the GitHub client for a fork; sessions swap their
	// client through it when the active fork changes. Nil in tests that
	// inject GH directly (fork switching then keeps the injected client).
	NewGH   func(fork settings.Fork) *gh.Client
	Version string
	// Prefs is the settings snapshot newDeps loaded; one-shot commands
	// read the active fork from it (sessions load their own).
	Prefs settings.Settings
	// DLSS, Launcher, and Covers are the same test seams GH is: nil
	// (production) lets newSession build the default clients; tests
	// inject fakes (or dead-URL clients, since a cover miss is tolerated).
	DLSS     *dlss.Client
	Launcher *launch.Launcher
	Covers   *covers.Covers
	// SteamRoot pins the Steam root for session-backed commands ("" =
	// auto-detect all libraries, the GUI/TUI behavior; the Scan command
	// already carries the same flag). Tests MUST pin it to the fixture
	// root so a scan never touches the real machine's libraries.
	SteamRoot string
}

// newGHFactory builds fork-scoped GitHub clients against base ("" = the
// production GitHub API), each with its own per-fork cache namespace.
func newGHFactory(cacheDir, base string) func(settings.Fork) *gh.Client {
	return func(fork settings.Fork) *gh.Client {
		dir := settings.BundleCacheDir(cacheDir, fork.Slug)
		if base != "" {
			return gh.NewForkWithBaseURL(nil, dir, base, fork.Slug, fork.AssetPattern)
		}
		return gh.NewFork(nil, dir, fork.Slug, fork.AssetPattern)
	}
}

// newDeps builds production dependencies. OM_DATA_DIR overrides the store
// root and OM_CACHE_DIR the cache root (testability); OM_GH_BASE_URL
// overrides the GitHub API base.
func newDeps(version string) (*Deps, error) {
	root := os.Getenv("OM_DATA_DIR")
	if root == "" {
		var err error
		root, err = store.DefaultRoot()
		if err != nil {
			return nil, err
		}
	}
	cacheDir := os.Getenv("OM_CACHE_DIR")
	if cacheDir == "" {
		cacheDir = defaultCacheRoot()
	}
	prefs, err := settings.Load(root)
	if err != nil {
		log.Warn().Err(err).Msg("settings unreadable, using defaults")
		prefs = settings.Defaults()
	}
	app.MigrateLegacyBundleCache(cacheDir)
	base := os.Getenv("OM_GH_BASE_URL")
	factory := newGHFactory(cacheDir, base)
	return &Deps{
		Out:      os.Stdout,
		ErrOut:   os.Stderr,
		Store:    store.New(root),
		DataRoot: root,
		CacheDir: cacheDir,
		GH:       factory(prefs.Active()),
		NewGH:    factory,
		Version:  version,
		Prefs:    prefs,
	}, nil
}

// defaultCacheRoot returns $XDG_CACHE_HOME/optiscaler-manager, falling back
// to ~/.cache/optiscaler-manager.
func defaultCacheRoot() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "optiscaler-manager")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "optiscaler-manager")
	}
	return filepath.Join(os.TempDir(), "optiscaler-manager")
}

// warnInterruptedCLI reports whether command gets the interrupted-install
// warning on stderr. The GUI and TUI surface it in-session (boot toast
// plus the persistent banner over actionable rows), so a stderr print
// would be invisible under the GUI or corrupt the TUI's alternate screen.
func warnInterruptedCLI(command string) bool {
	switch command {
	case "version", "gui", "tui":
		return false
	}
	return true
}

// checkInterrupted warns about installs left in in_progress/failed state.
// Such manifests mean the process died mid-transaction; only the user can
// choose repair/rollback/retry, so we surface and guide, never auto-delete.
func checkInterrupted(w io.Writer, st *store.Store) {
	manifests, err := st.List()
	if err != nil {
		log.Debug().Err(err).Msg("startup recovery: store unreadable")
		return
	}
	for _, m := range manifests {
		switch m.Status {
		case domain.StatusInProgress, domain.StatusFailed:
			fmt.Fprintf(w, "warning: interrupted install at %s (status %s); run `optiscaler-manager rollback %s` to restore, or `install` to retry\n",
				m.InstallDir, m.Status, m.GameRoot)
		}
	}
}
