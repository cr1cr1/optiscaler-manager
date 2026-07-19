// Package store persists install manifests and backup bytes under an
// external data root, keeping game directories free of manager state
// (docs/architecture.md: game dirs are never our database).
//
// Layout: manifests at <root>/manifests/<id>.json (atomic write, 0600),
// backups under <root>/backups/<id>/.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// Store is a concrete on-disk manifest/backup store rooted at an external
// data directory.
type Store struct {
	root string
}

// New returns a Store rooted at root. The root is created lazily on Save.
func New(root string) *Store {
	return &Store{root: root}
}

// DefaultRoot returns the platform data root: $XDG_DATA_HOME/optiscaler-manager,
// falling back to ~/.local/share/optiscaler-manager (Linux).
func DefaultRoot() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "optiscaler-manager"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("store: resolve default root: %w", err)
	}
	return filepath.Join(home, ".local", "share", "optiscaler-manager"), nil
}

// Save persists m, stamping UpdatedAt. The write is atomic: temp file in the
// manifests dir, 0600, then rename over <id>.json.
func (s *Store) Save(m *domain.Manifest) error {
	dir := s.manifestsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("store: create manifests dir: %w", err)
	}

	m.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("store: marshal manifest %q: %w", m.ID, err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*.json")
	if err != nil {
		return fmt.Errorf("store: create temp manifest: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("store: write temp manifest: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("store: chmod temp manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("store: close temp manifest: %w", err)
	}
	if err := os.Rename(tmpName, s.manifestPath(m.ID)); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("store: rename manifest %q: %w", m.ID, err)
	}
	return nil
}

// Load reads the manifest with the given ID.
func (s *Store) Load(id string) (*domain.Manifest, error) {
	data, err := os.ReadFile(s.manifestPath(id))
	if err != nil {
		return nil, fmt.Errorf("store: load manifest %q: %w", id, err)
	}
	var m domain.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("store: parse manifest %q: %w", id, err)
	}
	return &m, nil
}

// List returns every persisted manifest, sorted by ID. A missing manifests
// dir is an empty store, not an error.
func (s *Store) List() ([]*domain.Manifest, error) {
	entries, err := os.ReadDir(s.manifestsDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: list manifests: %w", err)
	}

	var out []*domain.Manifest
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		m, err := s.Load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// BackupDir returns the per-install backup directory for a manifest ID.
func (s *Store) BackupDir(id string) string {
	return filepath.Join(s.root, "backups", id)
}

// StagingDir returns the per-install extraction staging directory.
func (s *Store) StagingDir(id string) string {
	return filepath.Join(s.root, "staging", id)
}

// Delete removes the manifest with the given ID. A missing manifest is not
// an error (idempotent cleanup).
func (s *Store) Delete(id string) error {
	if err := os.Remove(s.manifestPath(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("store: delete manifest %q: %w", id, err)
	}
	return nil
}

func (s *Store) manifestsDir() string {
	return filepath.Join(s.root, "manifests")
}

func (s *Store) manifestPath(id string) string {
	return filepath.Join(s.manifestsDir(), id+".json")
}
