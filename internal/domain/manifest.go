package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// SchemaVersion is the current on-disk manifest schema version. Bump on any
// breaking change to Manifest's JSON shape.
const SchemaVersion = 1

// Status is the install state machine persisted in the manifest:
// in_progress → committed, or in_progress → failed → rolled_back
// (docs/safety.md). "planned" is deliberately in-memory only.
type Status string

const (
	StatusInProgress Status = "in_progress"
	StatusCommitted  Status = "committed"
	StatusFailed     Status = "failed"
	StatusRolledBack Status = "rolled_back"

	// StatusExternal marks an OptiScaler installation detected on disk that
	// this manager did not perform. It is derived at scan time and is NEVER
	// persisted to store manifests — the persisted set stays the four above.
	StatusExternal Status = "external"
)

// OpEntry is one planned or executed file operation. Op is one of
// "backup", "create", "rename", or "delete"; Path is relative to InstallDir.
type OpEntry struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

// OverwrittenEntry records a game file replaced during install, with the
// pre- and post-install hashes plus the manifest-relative backup location.
type OverwrittenEntry struct {
	Path            string `json:"path"`
	BackupRelPath   string `json:"backup_rel_path"`
	PreSHA256       string `json:"pre_sha256"`
	InstalledSHA256 string `json:"installed_sha256"`
}

// CreatedEntry records a file the install created. Uninstall deletes these
// only when the current SHA-256 still matches (docs/safety.md invariant 5).
type CreatedEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Manifest is the persisted record of one install, keyed by canonical
// install dir. It is written before the first destructive write and drives
// rollback and uninstall.
type Manifest struct {
	ID               string             `json:"id"`
	SchemaVersion    int                `json:"schema_version"`
	Status           Status             `json:"status"`
	GameRoot         string             `json:"game_root"`
	InstallDir       string             `json:"install_dir"` // canonical
	RequestedVersion string             `json:"requested_version"`
	Resolved         ResolvedAsset      `json:"resolved"`
	Ops              []OpEntry          `json:"ops"`
	Overwritten      []OverwrittenEntry `json:"overwritten"`
	Created          []CreatedEntry     `json:"created"`
	CreatedDirs      []string           `json:"created_dirs"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
	LastError        string             `json:"last_error"`
}

// ManifestID derives the stable, filesystem-safe manifest key for a
// canonical install dir: the first 16 hex chars of its SHA-256.
func ManifestID(canonicalInstallDir string) string {
	sum := sha256.Sum256([]byte(canonicalInstallDir))
	return hex.EncodeToString(sum[:])[:16]
}
