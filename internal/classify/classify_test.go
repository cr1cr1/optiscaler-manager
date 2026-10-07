package classify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// TestDirFilesSkipsDLSSBackups: optiscaler-manager's own in-game rollback
// stores (dlss-backups/, issue 022; optiscaler-backups/, issue 023) must
// never surface through DirFiles — their stale DLL copies would alias
// into version probing and injection-dir resolution as if they were
// active game components.
func TestDirFilesSkipsDLSSBackups(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{
		"nvngx_dlss.dll",
		"dlss-backups/20261007-130914_dlss-1.0.0/nvngx_dlss.dll",
		"optiscaler-backups/files/nvngx_dlss.dll",
	} {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := DirFiles(root)
	if len(got) != 1 || got[0].Path != filepath.Join(root, "nvngx_dlss.dll") {
		t.Fatalf("DirFiles = %v, want only the active nvngx_dlss.dll", got)
	}
}

// TestClassifyDetectsKnownComponentDLLs builds a fake game tree and asserts
// the exact component set Dir reports: known upscaler DLLs are detected
// (recursively, case-insensitively), unrelated DLLs are ignored, and noisy
// dirs like .git are skipped.
func TestClassifyDetectsKnownComponentDLLs(t *testing.T) {
	root := t.TempDir()

	// path -> expected component (nil means: must NOT be detected)
	files := []struct {
		path string
		want *domain.Component
	}{
		{"nvngx_dlss.dll", &domain.Component{Kind: domain.KindDLSS, DLL: "nvngx_dlss.dll"}},
		{"bin/NVNGX_DLSS.DLL", &domain.Component{Kind: domain.KindDLSS, DLL: "NVNGX_DLSS.DLL"}},
		{"nvngx_dlssg.dll", &domain.Component{Kind: domain.KindDLSSFG, DLL: "nvngx_dlssg.dll"}},
		{"bin/amd_fidelityfx_dx12.dll", &domain.Component{Kind: domain.KindFSR, DLL: "amd_fidelityfx_dx12.dll"}},
		{"bin/LIBXESS.DLL", &domain.Component{Kind: domain.KindXeSS, DLL: "LIBXESS.DLL"}},
		// future-name prefix rule
		{"bin/ffx_fsr4upscaler_x64.dll", &domain.Component{Kind: domain.KindFSR, DLL: "ffx_fsr4upscaler_x64.dll"}},
		// false-positive guards
		{"dxgi.dll", nil},
		{"d3d11.dll", nil},
		{"bin/version.dll", nil},
		{"readme.txt", nil},
		// noisy dirs are skipped
		{".git/nvngx_dlss.dll", nil},
		// optiscaler-manager's own in-game DLSS rollback store is not an
		// active game component (issue 022)
		{"dlss-backups/20261007-130914_dlss-1.0.0/nvngx_dlss.dll", nil},
		// nor is its OptiScaler install backup store (issue 023)
		{"optiscaler-backups/files/dxgi.dll", nil},
	}

	var want []domain.Component
	for _, f := range files {
		full := filepath.Join(root, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte("fake"), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
		if f.want != nil {
			t.Logf("expect detection: %s -> %s", f.path, f.want.Kind)
			want = append(want, *f.want)
		} else {
			t.Logf("expect no detection: %s", f.path)
		}
	}

	got := Dir(root)
	t.Logf("Dir returned %d components: %v", len(got), got)

	if len(got) != len(want) {
		t.Fatalf("got %d components %v, want %d %v", len(got), got, len(want), want)
	}

	// Dir must return deterministic sorted order (Kind, then DLL).
	for i := 1; i < len(got); i++ {
		if got[i-1].Kind > got[i].Kind ||
			(got[i-1].Kind == got[i].Kind && got[i-1].DLL > got[i].DLL) {
			t.Fatalf("result not sorted by (Kind, DLL): %v", got)
		}
	}

	// Compare as sets.
	seen := map[domain.Component]bool{}
	for _, c := range got {
		seen[c] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("missing expected component %+v", w)
		}
	}

	// Empty dir yields no components.
	if comps := Dir(t.TempDir()); len(comps) != 0 {
		t.Errorf("empty dir: got %v, want none", comps)
	}
}
