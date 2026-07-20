package domain_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// TestManifestJSONRoundTrip asserts that a fully populated Manifest survives
// a JSON marshal/unmarshal cycle unchanged, including nested entries. The
// manifest is the safety model's source of truth (docs/safety.md); a silent
// field loss here would corrupt rollback and uninstall decisions.
func TestManifestJSONRoundTrip(t *testing.T) {
	gameRoot := "/games/steam/steamapps/common/CyberGame"
	installDir := gameRoot + "/Game/Binaries/Win64"

	original := domain.Manifest{
		ID:               domain.ManifestID(installDir),
		SchemaVersion:    domain.SchemaVersion,
		Status:           domain.StatusCommitted,
		GameRoot:         gameRoot,
		InstallDir:       installDir,
		RequestedVersion: "0.9.4",
		Resolved: domain.ResolvedAsset{
			AssetName: "Optiscaler_0.9.4-final_20260701_MM.7z",
			Version:   "0.9.4",
			SHA256:    "0123456789abcdef",
		},
		Ops: []domain.OpEntry{
			{Op: "backup", Path: "dxgi.dll"},
			{Op: "rename", Path: "OptiScaler.dll"},
			{Op: "create", Path: "OptiScaler.ini"},
			{Op: "delete", Path: "stale.dll"},
		},
		Overwritten: []domain.OverwrittenEntry{
			{
				Path:            "dxgi.dll",
				BackupRelPath:   "dxgi.dll",
				PreSHA256:       "aaaa",
				InstalledSHA256: "bbbb",
			},
		},
		Created: []domain.CreatedEntry{
			{Path: "OptiScaler.ini", SHA256: "cccc"},
			{Path: "amd_fidelityfx_dx12.dll", SHA256: "dddd"},
		},
		CreatedDirs: []string{"DlssOverrides"},
		CreatedAt:   time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 7, 19, 12, 5, 30, 0, time.UTC),
		LastError:   "",
	}

	t.Logf("manifest ID for %q: %q", installDir, original.ID)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded domain.Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !reflect.DeepEqual(original, decoded) {
		t.Errorf("round trip mismatch:\n got: %+v\nwant: %+v", decoded, original)
	}
	t.Logf("round trip preserved %d ops, %d overwritten, %d created, %d created dirs",
		len(decoded.Ops), len(decoded.Overwritten), len(decoded.Created), len(decoded.CreatedDirs))
}

func TestPersistedStatusSet(t *testing.T) {
	persisted := []domain.Status{domain.StatusInProgress, domain.StatusCommitted, domain.StatusFailed, domain.StatusRolledBack}
	for _, s := range persisted {
		if s == domain.StatusExternal {
			t.Errorf("StatusExternal must never collide with persisted status %q", s)
		}
	}
	if domain.StatusExternal == "" {
		t.Error("StatusExternal must be a non-empty derived status")
	}
}
