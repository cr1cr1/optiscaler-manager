package pever

import (
	"os"
	"path/filepath"
	"testing"
)

// --- DetectOptiScaler fixture helpers ------------------------------------

func writeCandidate(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// identityPE builds a PE carrying only the given StringFileInfo identity
// entries (no fixed FILEVERSION, no ProductVersion).
func identityPE(pe32plus bool, keyVals ...[2]string) []byte {
	parts := make([][]byte, 0, len(keyVals))
	for _, kv := range keyVals {
		parts = append(parts, stringInfoString(kv[0], kv[1]))
	}
	return buildPE(pe32plus, concatStringStructs(parts...))
}

// identityVersionPE builds a PE with identity entries plus a fixed
// FILEVERSION quad.
func identityVersionPE(pe32plus bool, maj, min, patch, build uint16, keyVals ...[2]string) []byte {
	res := append([]byte(nil), fixedFileInfo(maj, min, patch, build)...)
	for _, kv := range keyVals {
		res = append(res, stringInfoString(kv[0], kv[1])...)
		for len(res)%4 != 0 {
			res = append(res, 0)
		}
	}
	return buildPE(pe32plus, res)
}

// --- tests ---------------------------------------------------------------

func TestDetectOptiScaler_FoundByOriginalFilename(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "dxgi.dll", identityPE(false, [2]string{"OriginalFilename", "OptiScaler.dll"}))
	found, _ := DetectOptiScaler(dir)
	if !found {
		t.Error("want found via OriginalFilename")
	}
}

func TestDetectOptiScaler_FoundByProductName(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "dxgi.dll", identityPE(true, [2]string{"ProductName", "OptiScaler"}))
	found, _ := DetectOptiScaler(dir)
	if !found {
		t.Error("want found via ProductName")
	}
}

func TestDetectOptiScaler_FoundByCompanyName(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "winmm.dll", identityPE(false, [2]string{"CompanyName", "OptiScaler"}))
	found, _ := DetectOptiScaler(dir)
	if !found {
		t.Error("want found via CompanyName")
	}
}

func TestDetectOptiScaler_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "dxgi.dll", identityPE(false, [2]string{"ProductName", "OPTISCALER"}))
	found, _ := DetectOptiScaler(dir)
	if !found {
		t.Error("want found for case-insensitive match")
	}
}

func TestDetectOptiScaler_DXVKNotMatched_ContinuesScanning(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "dxgi.dll", identityPE(false, [2]string{"ProductName", "DXVK"}))
	writeCandidate(t, dir, "winmm.dll", identityPE(false, [2]string{"ProductName", "OptiScaler"}))
	found, _ := DetectOptiScaler(dir)
	if !found {
		t.Error("want found via winmm.dll; a DXVK dxgi.dll must not stop the scan")
	}
}

func TestDetectOptiScaler_DXVKOnly_NotFound(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "dxgi.dll", identityPE(false, [2]string{"ProductName", "DXVK"}))
	found, _ := DetectOptiScaler(dir)
	if found {
		t.Error("want not found for DXVK-only directory")
	}
}

func TestDetectOptiScaler_NoCandidates(t *testing.T) {
	found, version := DetectOptiScaler(t.TempDir())
	if found {
		t.Error("want not found in empty directory")
	}
	if version != "" {
		t.Errorf("version = %q, want %q", version, "")
	}
}

func TestDetectOptiScaler_NonPECandidateSkipped(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "dxgi.dll", []byte("this is not a PE file, just garbage bytes"))
	writeCandidate(t, dir, "version.dll", identityPE(false, [2]string{"ProductName", "OptiScaler"}))
	found, _ := DetectOptiScaler(dir)
	if !found {
		t.Error("want found via version.dll; a garbage dxgi.dll must be skipped")
	}
}

func TestDetectOptiScaler_UnrenamedDLL(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "OptiScaler.dll", identityPE(true, [2]string{"ProductName", "OptiScaler"}))
	found, _ := DetectOptiScaler(dir)
	if !found {
		t.Error("want found for unrenamed OptiScaler.dll")
	}
}

func TestDetectOptiScaler_AllCandidateNames(t *testing.T) {
	names := []string{
		"dxgi.dll", "OptiScaler.dll", "winmm.dll", "dbghelp.dll",
		"version.dll", "wininet.dll", "winhttp.dll", "d3d12.dll",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeCandidate(t, dir, name, identityPE(false, [2]string{"ProductName", "OptiScaler"}))
			found, _ := DetectOptiScaler(dir)
			if !found {
				t.Errorf("want found with only %s present", name)
			}
		})
	}
}

func TestDetectOptiScaler_VersionChainPrefersManifestThenLogThenPE(t *testing.T) {
	peWithVersion := identityVersionPE(false, 0, 7, 2, 0, [2]string{"ProductName", "OptiScaler"})

	t.Run("manifest wins over log and PE", func(t *testing.T) {
		dir := t.TempDir()
		writeCandidate(t, dir, "OptiScaler.dll", peWithVersion)
		writeCandidate(t, dir, "manifest.json", []byte(`{"version":"0.9.4"}`))
		writeCandidate(t, dir, "OptiScaler.log", []byte("noise\nOptiScaler v0.8.1-pre3 (build abc)\n"))
		found, version := DetectOptiScaler(dir)
		if !found {
			t.Fatal("want found")
		}
		if version != "0.9.4" {
			t.Errorf("version = %q, want %q (manifest)", version, "0.9.4")
		}
	})

	t.Run("log wins over PE", func(t *testing.T) {
		dir := t.TempDir()
		writeCandidate(t, dir, "OptiScaler.dll", peWithVersion)
		writeCandidate(t, dir, "OptiScaler.log", []byte("noise\nOptiScaler v0.8.1-pre3 (build abc)\n"))
		found, version := DetectOptiScaler(dir)
		if !found {
			t.Fatal("want found")
		}
		if version != "0.8.1-pre3" {
			t.Errorf("version = %q, want %q (log banner)", version, "0.8.1-pre3")
		}
	})

	t.Run("PE version as last resort", func(t *testing.T) {
		dir := t.TempDir()
		writeCandidate(t, dir, "OptiScaler.dll", peWithVersion)
		found, version := DetectOptiScaler(dir)
		if !found {
			t.Fatal("want found")
		}
		if version != "0.7.2.0" {
			t.Errorf("version = %q, want %q (PE fixed quad)", version, "0.7.2.0")
		}
	})
}

func TestDetectOptiScaler_VersionEmptyWhenNoEvidence(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "OptiScaler.dll", identityPE(false, [2]string{"ProductName", "OptiScaler"}))
	found, version := DetectOptiScaler(dir)
	if !found {
		t.Fatal("want found")
	}
	if version != "" {
		t.Errorf("version = %q, want %q (no manifest, log, or PE version)", version, "")
	}
}
