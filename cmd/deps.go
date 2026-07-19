package optiscalermanager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/gh"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// Deps carries everything a subcommand needs, injected by the root command
// (and by tests, which substitute buffers, temp stores, and httptest-backed
// GitHub clients).
type Deps struct {
	Out      io.Writer
	ErrOut   io.Writer
	Store    *store.Store
	CacheDir string
	GH       *gh.Client
	Version  string
}

// newDeps builds production dependencies. OM_DATA_DIR overrides the store
// root (testability); OM_GH_BASE_URL overrides the GitHub API base.
func newDeps(version string) (*Deps, error) {
	root := os.Getenv("OM_DATA_DIR")
	if root == "" {
		var err error
		root, err = store.DefaultRoot()
		if err != nil {
			return nil, err
		}
	}
	cacheDir := filepath.Join(root, "cache")
	var ghClient *gh.Client
	if base := os.Getenv("OM_GH_BASE_URL"); base != "" {
		ghClient = gh.NewWithBaseURL(nil, cacheDir, base)
	} else {
		ghClient = gh.New(nil, cacheDir)
	}
	return &Deps{
		Out:      os.Stdout,
		ErrOut:   os.Stderr,
		Store:    store.New(root),
		CacheDir: cacheDir,
		GH:       ghClient,
		Version:  version,
	}, nil
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
