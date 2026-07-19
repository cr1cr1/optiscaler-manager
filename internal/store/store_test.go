package store_test

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

func manifestFor(installDir string) *domain.Manifest {
	ts := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	return &domain.Manifest{
		ID:            domain.ManifestID(installDir),
		SchemaVersion: domain.SchemaVersion,
		Status:        domain.StatusInProgress,
		GameRoot:      installDir,
		InstallDir:    installDir,
		CreatedAt:     ts,
		UpdatedAt:     ts,
	}
}

// TestStoreSaveLoadListManifests covers the store contract the installer
// relies on: a manifest persisted before the first destructive write must be
// loadable afterwards, List must surface every persisted manifest, and Save
// must stamp UpdatedAt so the UI can order by recency.
func TestStoreSaveLoadListManifests(t *testing.T) {
	root := t.TempDir()
	s := store.New(root)

	m1 := manifestFor("/games/a")
	m2 := manifestFor("/games/b")

	if err := s.Save(m1); err != nil {
		t.Fatalf("save m1: %v", err)
	}
	if err := s.Save(m2); err != nil {
		t.Fatalf("save m2: %v", err)
	}
	t.Logf("saved manifests %q and %q under %q", m1.ID, m2.ID, root)

	got1, err := s.Load(m1.ID)
	if err != nil {
		t.Fatalf("load m1: %v", err)
	}
	if !reflect.DeepEqual(m1, got1) {
		t.Errorf("load m1 mismatch:\n got: %+v\nwant: %+v", got1, m1)
	}
	got2, err := s.Load(m2.ID)
	if err != nil {
		t.Fatalf("load m2: %v", err)
	}
	if !reflect.DeepEqual(m2, got2) {
		t.Errorf("load m2 mismatch:\n got: %+v\nwant: %+v", got2, m2)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	ids := make([]string, 0, len(list))
	for _, m := range list {
		ids = append(ids, m.ID)
	}
	sort.Strings(ids)
	wantIDs := []string{m1.ID, m2.ID}
	sort.Strings(wantIDs)
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Errorf("list IDs = %v, want %v", ids, wantIDs)
	}
	t.Logf("list returned %d manifests: %v", len(list), ids)

	// Re-save must refresh UpdatedAt; the installer re-saves the same manifest
	// as the state machine advances.
	sentinel := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	m1.Status = domain.StatusCommitted
	m1.UpdatedAt = sentinel
	if err := s.Save(m1); err != nil {
		t.Fatalf("re-save m1: %v", err)
	}
	again, err := s.Load(m1.ID)
	if err != nil {
		t.Fatalf("reload m1: %v", err)
	}
	if !again.UpdatedAt.After(sentinel) {
		t.Errorf("UpdatedAt not refreshed on re-save: got %v, sentinel %v", again.UpdatedAt, sentinel)
	}
	if again.Status != domain.StatusCommitted {
		t.Errorf("status = %q, want %q", again.Status, domain.StatusCommitted)
	}
	t.Logf("re-save refreshed UpdatedAt to %v", again.UpdatedAt)

	if _, err := s.Load("0000000000000000"); err == nil {
		t.Error("load of unknown ID: expected error, got nil")
	} else {
		t.Logf("unknown ID load errored as expected: %v", err)
	}

	if d := s.BackupDir(m1.ID); d != filepath.Join(root, "backups", m1.ID) {
		t.Errorf("BackupDir = %q, want %q", d, filepath.Join(root, "backups", m1.ID))
	}
}
