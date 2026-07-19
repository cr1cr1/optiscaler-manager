package installer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallWritesCuratedINI(t *testing.T) {
	root, bin, st := newGame(t)
	m, err := Install(context.Background(), st, request(root, bin))
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	ini := filepath.Join(bin, "OptiScaler.ini")
	data, err := os.ReadFile(ini)
	if err != nil {
		t.Fatalf("curated ini missing: %v", err)
	}
	if !strings.Contains(string(data), "Dx12Upscaler=auto") {
		t.Errorf("ini was not replaced with curated defaults:\n%s", data)
	}

	// Manifest records the curated bytes, not the bundle ones.
	for _, c := range m.Created {
		if c.Path == ini {
			if c.SHA256 != sha(t, ini) {
				t.Errorf("manifest hash %q != curated ini hash", c.SHA256)
			}
			t.Logf("manifest tracks curated ini: %s", c.SHA256)
			return
		}
	}
	t.Error("no created entry for OptiScaler.ini")
}

func TestEACProtectedDetectsStartProtectedGame(t *testing.T) {
	root, _, _ := newGame(t)
	if EACProtected(root) {
		t.Error("clean game root reported as EAC-protected")
	}
	writeFile(t, filepath.Join(root, "start_protected_game.exe"), "EAC")
	if !EACProtected(root) {
		t.Error("EAC marker not detected")
	}
	t.Logf("EAC detection works on %s", root)
}
