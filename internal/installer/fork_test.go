package installer

import (
	"context"
	"testing"
)

// The manifest records which distribution fork the bundle came from, so
// upgrade checks can compare within the same fork later. Empty stays
// empty: the app layer normalizes to the upstream slug; legacy manifests
// (no key) read as upstream.
func TestManifestRecordsFork(t *testing.T) {
	root, bin, st := newGame(t)
	req := request(root, bin)
	req.Fork = "jlrouzies-fr/OptiScaler-DLSSNR-PreSR-Multipass"
	m, err := Install(context.Background(), st, req)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if m.Fork != req.Fork {
		t.Errorf("manifest Fork = %q, want %q", m.Fork, req.Fork)
	}

	root2, bin2, st2 := newGame(t)
	m2, err := Install(context.Background(), st2, request(root2, bin2))
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if m2.Fork != "" {
		t.Errorf("manifest Fork = %q, want empty when the request carries none", m2.Fork)
	}
}
