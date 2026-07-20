package optiscalermanager

import (
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/protondb"
	"github.com/cr1cr1/optiscaler-manager/internal/steam"
)

// TestOnlineClientsWiring: newSession's online-client builder returns real
// Steam/ProtonDB clients (typed, non-nil) so production scans actually run
// the online enrichment phase instead of silently skipping it.
func TestOnlineClientsWiring(t *testing.T) {
	sc, pc := onlineClients(filepath.Join(t.TempDir(), "cache"), "v0.0.0-test")
	if sc == nil {
		t.Error("onlineClients returned a nil Steam client")
	}
	if pc == nil {
		t.Error("onlineClients returned a nil ProtonDB client")
	}
	var _ *steam.Client = sc
	var _ *protondb.Client = pc
}
