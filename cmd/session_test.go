package optiscalermanager

import (
	"path/filepath"
	"testing"
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
}
