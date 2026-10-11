package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

func writeReconFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func reconManifest(t *testing.T, st *store.Store, root, bin string) {
	t.Helper()
	can, err := canonicalDir(bin)
	if err != nil {
		t.Fatal(err)
	}
	sha := func(p string) string {
		h, err := fileSHA256(p)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	m := &domain.Manifest{
		ID:            domain.ManifestID(can),
		SchemaVersion: domain.SchemaVersion,
		Status:        domain.StatusCommitted,
		GameRoot:      root,
		InstallDir:    can,
		Resolved:      domain.ResolvedAsset{AssetName: "Optiscaler_test.7z", Version: "v1.2.3"},
		Fork:          "owner/repo",
		Created: []domain.CreatedEntry{
			{Path: filepath.Join(bin, "a.dll"), SHA256: sha(filepath.Join(bin, "a.dll"))},
			{Path: filepath.Join(bin, "b.dll"), SHA256: sha(filepath.Join(bin, "b.dll"))},
			{Path: filepath.Join(bin, "c-missing.dll"), SHA256: "0c0ffee"},
		},
		Overwritten: []domain.OverwrittenEntry{
			{Path: filepath.Join(bin, "d.dll"), InstalledSHA256: sha(filepath.Join(bin, "d.dll")), BackupRelPath: "d.dll", PreSHA256: "00"},
		},
	}
	if err := st.Save(m); err != nil {
		t.Fatal(err)
	}
}

// TestReconcileInstall_ManagedCounts: directory facts alone drive the
// verdict — verified/changed/missing counts from a hand-built manifest
// (issue 079).
func TestReconcileInstall_ManagedCounts(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	writeReconFile(t, filepath.Join(bin, "game.exe"), "MZfake")
	writeReconFile(t, filepath.Join(bin, "a.dll"), "A-bytes")
	writeReconFile(t, filepath.Join(bin, "b.dll"), "B-bytes")
	writeReconFile(t, filepath.Join(bin, "d.dll"), "D-bytes")
	st := store.New(t.TempDir())
	reconManifest(t, st, root, bin)

	truth := ReconcileInstall(st, root, bin)
	if !truth.Managed {
		t.Fatal("Managed = false, want true with a committed manifest")
	}
	if truth.Version != "v1.2.3" || truth.Fork != "owner/repo" {
		t.Errorf("version/fork = %q/%q, want v1.2.3/owner/repo", truth.Version, truth.Fork)
	}
	if truth.Tracked != 4 || truth.Verified != 3 || truth.Changed != 0 || truth.Missing != 1 {
		t.Errorf("counts = %d/%d/%d/%d, want 4/3/0/1 (tracked/verified/changed/missing)",
			truth.Tracked, truth.Verified, truth.Changed, truth.Missing)
	}

	// The Witcher 3 scenario: foreign bytes land over a managed file.
	writeReconFile(t, filepath.Join(bin, "b.dll"), "B-CHANGED")
	truth = ReconcileInstall(st, root, bin)
	if truth.Changed != 1 || truth.Verified != 2 || truth.Missing != 1 {
		t.Errorf("after foreign edit counts = %d/%d/%d, want verified 2 changed 1 missing 1",
			truth.Verified, truth.Changed, truth.Missing)
	}
	t.Log("reconcile: directory facts drive the verdict")
}

// TestReconcileInstall_Unmanaged: no manifest means not managed, but the
// parked hook is still reported from the directory (issue 079).
func TestReconcileInstall_Unmanaged(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	writeReconFile(t, filepath.Join(bin, "game.exe"), "MZfake")
	writeReconFile(t, filepath.Join(bin, "dxgi.dll.disabled"), "parked")
	st := store.New(t.TempDir())

	truth := ReconcileInstall(st, root, bin)
	if truth.Managed {
		t.Error("Managed = true without a manifest")
	}
	if truth.Tracked != 0 {
		t.Errorf("Tracked = %d, want 0 without a manifest", truth.Tracked)
	}
	if !truth.HookDisabled {
		t.Error("HookDisabled = false with dxgi.dll.disabled on disk")
	}
	if truth.Hook {
		t.Error("Hook = true with no branded hook on disk")
	}
	t.Log("reconcile: unmanaged dir reads as not-managed with parked hook")
}
