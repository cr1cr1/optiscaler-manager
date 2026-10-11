package app

import (
	"errors"
	"io/fs"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/pever"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// InstallTruth is the install state verified live against the game
// directory (issue 079). The manager's store records are a claim; this is
// the directory's word: every tracked file re-hashed, the injection hook
// and its disabled twin probed. Details panes render from it instead of
// trusting the cached row status.
type InstallTruth struct {
	// Managed reports a committed store manifest for the game. Unmanaged
	// games (external or clean) read every count as zero.
	Managed bool
	Version string // manifest resolved version ("" when unmanaged)
	Fork    string // manifest fork slug ("" when unmanaged)
	// Tracked/Verified/Changed/Missing partition the manifest's created +
	// overwritten entries by current directory facts.
	Tracked  int
	Verified int
	Changed  int
	Missing  int
	// Hook reports a branded injection DLL in the injection dir;
	// HookVersion its detected version. HookDisabled reports a parked
	// (.disabled et al) hook.
	Hook         bool
	HookVersion  string
	HookDisabled bool
}

// ReconcileInstall verifies a game root against its actual directory and
// returns the directory's verdict. It never writes: missing manifests,
// missing files, and hash mismatches are reported, not repaired. st may
// be nil (reads as unmanaged); injectionDir "" skips the hook probes.
func ReconcileInstall(st *store.Store, gameRoot, injectionDir string) InstallTruth {
	var truth InstallTruth
	if st != nil {
		if id, _, err := ManifestIDFor(gameRoot); err == nil {
			if m, err := st.Load(id); err == nil && m != nil &&
				m.Status == domain.StatusCommitted {
				truth.Managed = true
				truth.Version = m.Resolved.Version
				truth.Fork = m.Fork
				truth.reconcileEntries(m)
			}
		}
	}
	if injectionDir != "" {
		if found, version := pever.DetectOptiScaler(injectionDir); found {
			truth.Hook = true
			truth.HookVersion = version
		}
		if _, f := pever.DisabledHook(injectionDir); f != "" {
			truth.HookDisabled = true
		}
	}
	return truth
}

// reconcileEntries partitions the manifest's tracked files by hashing each
// against the directory: present-and-matching is verified, present-but-
// different is changed (the foreign-modification the uninstall gates
// refuse on), absent is missing.
func (t *InstallTruth) reconcileEntries(m *domain.Manifest) {
	check := func(path, want string) {
		t.Tracked++
		got, err := fileSHA256(path)
		switch {
		case err == nil && (want == "" || got == want):
			t.Verified++
		case err == nil:
			t.Changed++
		case errors.Is(err, fs.ErrNotExist):
			t.Missing++
		default:
			// Unreadable is indistinguishable from tampered for display:
			// surface it as changed, never as verified.
			t.Changed++
		}
	}
	for _, c := range m.Created {
		check(c.Path, c.SHA256)
	}
	for _, ow := range m.Overwritten {
		check(ow.Path, ow.InstalledSHA256)
	}
}
